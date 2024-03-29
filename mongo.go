package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"sync"
	"time"

	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"
)

type mongoConfig struct {
	host       string
	replicaSet string
	port       int
	user       string
	authSource string
	password   string
	database   string
	caCert     string
	tls        bool
	clientCert string
}

func (mongo mongoConfig) dump(sb s3Backend, publicKeyPath, database string) error {
	log.Info("Mongo dump started")
	today := time.Now().Format("20060102150405")
	mongo.database = database
	dumpCommand := buildDumpCommand(mongo)
	log.Debugln(dumpCommand)

	cmd := exec.Command("sh", "-c", dumpCommand)

	var out bytes.Buffer
	cmd.Stdout = &out

	var errMsg bytes.Buffer
	cmd.Stderr = &errMsg

	err := cmd.Run()
	if err != nil {
		return err
	}

	log.Debug("Mongo dump command successfully executed")

	wg := sync.WaitGroup{}
	mongoArchive := today + "-" + database + ".archive"
	wr, err := sb.NewFileWriter(mongoArchive, &wg)
	if err != nil {
		return fmt.Errorf("Could not open backup file for writing: %s", err)
	}

	log.Debugf("Mongo archive file %v ready for writing", mongoArchive)

	privateKey, publicKeyList, err := getKeys(publicKeyPath)
	if err != nil {
		return fmt.Errorf("Could not retrieve public key or generate private key: %s", err)
	}

	log.Debug("Public key retrieved and private key successfully created")

	e, err := newEncryptor(publicKeyList, privateKey, wr)
	if err != nil {
		return fmt.Errorf("Could not initialize encryptor: %s", err)
	}

	log.Debug("Encryption initialized")

	c, err := newCompressor(e)
	if err != nil {
		return fmt.Errorf("Could not initialize compressor: %s", err)
	}

	log.Debug("Compression initialized")

	_, err = c.Write(out.Bytes())
	if err != nil {
		log.Errorf("Could not encrypt/write: %s", err)
	}

	if err := c.Close(); err != nil {
		log.Errorf("Could not close compressor: %v", err)
	}

	if err := e.Close(); err != nil {
		log.Errorf("Could not close encryptor: %v", err)
	}

	if err := wr.Close(); err != nil {
		log.Errorf("Could not close destination file: %v", err)
	}
	wg.Wait()

	log.Info("Mongo archive is compressed and encrypted")

	return nil
}

func (mongo mongoConfig) restore(sb s3Backend, privateKeyPath, archive, c4ghPassword string) error {
	log.Info("Start restoration from mongo archive")
	fr, err := sb.NewFileReader(archive)
	if err != nil {
		return err
	}
	defer fr.Close()

	log.Debug("Read mongo file")

	privateKey, err := getPrivateKey(privateKeyPath, c4ghPassword)
	if err != nil {
		return fmt.Errorf("Could not retrieve private key: %s", err)
	}

	log.Debug("Private key retrieved")

	r, err := newDecryptor(privateKey, fr)
	if err != nil {
		return fmt.Errorf("Could not initialise decryptor: %s", err)
	}

	log.Debug("Decryption initialized")

	d, err := newDecompressor(r)
	if err != nil {
		return fmt.Errorf("Could not initialise decompressor: %s", err)
	}

	log.Debug("Decompression initialized")

	restoreCommand := buildRestoreCommand(mongo)
	log.Debugln(restoreCommand)
	cmd := exec.Command("sh", "-c", restoreCommand)

	var in bytes.Buffer
	cmd.Stdin = &in

	_, err = in.ReadFrom(d)
	if err != nil {
		return fmt.Errorf("Could not read datastream: %s", err)
	}

	if err := d.Close(); err != nil {
		log.Errorf("Could not close decompressor: %v", err)
	}

	if err := r.Close(); err != nil {
		log.Errorf("Could not close decryptor: %v", err)
	}

	log.Debug("Data read successfully")

	var errMsg bytes.Buffer
	cmd.Stderr = &errMsg

	err = cmd.Run()
	if err != nil {
		return err
	}

	log.Debug("Importing mongo data finished")

	return nil
}

func buildRestoreCommand(mongo mongoConfig) string {
	cmd := fmt.Sprintf("mongorestore --uri='mongodb://%s:%s@%s/?authSource=admin", mongo.user, mongo.password, mongo.host)

	if mongo.replicaSet != "" {
		cmd += fmt.Sprintf("&replicaSet=%s'", mongo.replicaSet)
	} else {
		cmd += "'"
	}
	if mongo.tls {
		cmd += fmt.Sprintf(" --ssl --sslCAFile=%s --sslPEMKeyFile=%s", mongo.caCert, mongo.clientCert)
	}
	cmd += " --archive"

	return cmd
}

func buildDumpCommand(mongo mongoConfig) string {
	cmd := fmt.Sprintf("mongodump --uri='mongodb://%s:%s@%s/%s?authSource=admin", mongo.user, mongo.password, mongo.host, mongo.database)

	if mongo.replicaSet != "" {
		cmd += fmt.Sprintf("&replicaSet=%s&readPreference=secondary'", mongo.replicaSet)
	} else {
		cmd += "'"
	}

	if mongo.tls {
		cmd += fmt.Sprintf(" --ssl --sslCAFile=%s --sslPEMKeyFile=%s", mongo.caCert, mongo.clientCert)
	}
	cmd += " --archive"

	return cmd
}
