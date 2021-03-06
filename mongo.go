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

func (mongo mongoConfig) dump(sb s3Backend, keyPath, database string) error {
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
		log.Errorf(errMsg.String())
		return err
	}

	wg := sync.WaitGroup{}
	wr, err := sb.NewFileWriter(today+"-"+database+".archive", &wg)
	if err != nil {
		log.Errorf("Could not open backup file for writing: %v", err)
		return err
	}

	key := getKey(keyPath)
	e, err := newEncryptor(key, wr)
	if err != nil {
		log.Errorf("Could not initialize encryptor: (%v)", err)
		return err
	}

	c, err := newCompressor(key, e)
	if err != nil {
		log.Errorf("Could not initialize compressor: (%v)", err)
		return err
	}

	_, err = c.Write(out.Bytes())
	if err != nil {
		log.Errorf("Could not encrypt/write: %s", err)
		return err
	}

	c.Close()
	wr.Close()
	wg.Wait()

	return nil
}

func (mongo mongoConfig) restore(sb s3Backend, keyPath, archive string) error {

	fr, err := sb.NewFileReader(archive)
	if err != nil {
		log.Error(err)
		return err
	}
	defer fr.Close()

	key := getKey(keyPath)
	r, err := newDecryptor(key, fr)
	if err != nil {
		log.Error("Could not initialise decryptor", err)
		return err
	}
	d, err := newDecompressor(key, r)
	if err != nil {
		log.Error("Could not initialise decompressor", err)
		return err

	}

	restoreCommand := buildRestoreCommand(mongo)
	log.Debugln(restoreCommand)
	cmd := exec.Command("sh", "-c", restoreCommand)

	var in bytes.Buffer
	cmd.Stdin = &in

	_, err = in.ReadFrom(d)
	if err != nil {
		log.Error("Could not read datastream", err)
		return err

	}

	var errMsg bytes.Buffer
	cmd.Stderr = &errMsg

	err = cmd.Run()
	if err != nil {
		log.Errorf(errMsg.String())
		return err
	}

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
