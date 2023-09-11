package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"
)

// DBConf stores information about the database backend
type DBConf struct {
	host       string
	port       int
	user       string
	password   string
	database   string
	caCert     string
	sslMode    string
	clientCert string
	clientKey  string
}

// Basebackup function:
// - gets an identical copy of the pg database (pg_data)
// - verifies the backup
// - tars the copy
// - compresses the encrypted file
// - gets the key and encrypts the tar file
// - puts the encrypted and compressed file in S3
func (db DBConf) basebackup(sb s3Backend, keyPath string) error {
	log.Info("Basebackup started")
	today := time.Now().Format("20060102150405")
	destDir := "db-backup"
	dbURI := buildConnInfo(db)
	cmd := exec.Command("pg_basebackup", dbURI, "-F", "p", "-D", destDir)

	var errMsg bytes.Buffer
	cmd.Stderr = &errMsg

	err := cmd.Run()
	if err != nil {
		return err
	}

	log.Debugf("Backup command successfully executed in directory: %v", destDir)

	cmd = exec.Command("pg_verifybackup", destDir)

	cmd.Stderr = &errMsg

	err = cmd.Run()
	if err != nil {
		return err
	}

	log.Debug("Verify backup command successfully executed")

	cmd = exec.Command("tar", "-cvf", destDir+".tar", destDir)

	cmd.Stderr = &errMsg

	err = cmd.Run()
	if err != nil {
		return err
	}

	log.Debugf("%v.tar file created", destDir)

	fileName := today + "-" + db.database + ".enc"
	wg := sync.WaitGroup{}
	wr, err := sb.NewFileWriter(fileName, &wg)
	if err != nil {
		log.Error("Could not open backup file for writing")

		return err
	}

	log.Debugf("Backup file %v ready for writing", fileName)

	privateKey, publicKeyList := getKeys(keyPath)
	e, err := newEncryptor(publicKeyList, privateKey, wr)
	if err != nil {
		log.Error("Could not initialize encryptor")

		return err
	}

	log.Debug("Encryption initialized")

	c, err := newCompressor(e)
	if err != nil {
		log.Error("Could not initialize compressor")

		return err
	}

	log.Debug("Compression initialized")

	sourceFileName := destDir + ".tar"
	data, err := os.ReadFile(sourceFileName)
	if err != nil {
		log.Errorf("Error in reading source data: %v", err)
	}
	_, err = c.Write(data)
	if err != nil {
		log.Errorf("Error in writer: %v", err)
	}

	err = c.Close()
	if err != nil {
		log.Errorf("Could not close compressor: %v", err)
	}

	err = e.Close()
	if err != nil {
		log.Errorf("Could not close encryptor: %v", err)
	}

	err = wr.Close()
	if err != nil {
		log.Errorf("Could not close destination file: %v", err)
	}
	wg.Wait()

	log.Info("Backup data are compressed and encrypted")

	return nil
}

func (db DBConf) dump(sb s3Backend, keyPath string) error {
	log.Info("Dump backup started")
	today := time.Now().Format("20060102150405")
	dbURI := buildConnInfo(db)
	cmd := exec.Command("pg_dump", dbURI, "-xF", "tar")

	var out bytes.Buffer
	cmd.Stdout = &out

	var errMsg bytes.Buffer
	cmd.Stderr = &errMsg

	err := cmd.Run()
	if err != nil {
		return err
	}

	log.Debug("Dump command successfully executed")

	wg := sync.WaitGroup{}
	dumpFile := today + "-" + db.database + ".sqldump"
	wr, err := sb.NewFileWriter(dumpFile, &wg)
	if err != nil {
		log.Error("Could not open backup file for writing")

		return err
	}

	log.Debugf("Dump file %v ready for writing", dumpFile)

	privateKey, publicKeyList := getKeys(keyPath)

	e, err := newEncryptor(publicKeyList, privateKey, wr)
	if err != nil {
		log.Error("Could not initialize encryptor")

		return err
	}

	log.Debug("Encryption initialized")

	c, err := newCompressor(e)
	if err != nil {
		log.Error("Could not initialize compressor")

		return err
	}

	log.Debug("Compression initialized")

	_, err = c.Write(out.Bytes())
	if err != nil {
		log.Error("Could not encrypt/write")

		return err
	}

	err = c.Close()
	if err != nil {
		log.Errorf("Could not close compressor: %v", err)
	}

	err = e.Close()
	if err != nil {
		log.Errorf("Could not close encryptor: %v", err)
	}

	err = wr.Close()
	if err != nil {
		log.Errorf("Could not close destination file: %v", err)
	}
	wg.Wait()

	log.Info("Dump data are compressed and encrypted")

	return nil
}

// BasebackupUnpack function:
// - gets the key to decrypt the pg_data
// - decrypts and decompress the data
// - untar the data
// - puts the db copy in the running container
func (db DBConf) baseBackupUnpack(sb s3Backend, keyPath, backupTar string) error {
	log.Info("Unpacking basebackup data started")
	localTar, err := os.Create("/home/backup.tar")
	if err != nil {
		log.Errorf("Error in creating file: %v", err)
	}

	log.Debug("File created")

	fr, err := sb.NewFileReader(backupTar)
	if err != nil {
		return err
	}
	defer fr.Close()

	log.Debug("Data ready for unpacking")

	key := getKey(keyPath)
	r, err := newDecryptor(key, fr)
	if err != nil {
		log.Error("Could not initialise decryptor")

		return err
	}

	log.Debug("Decryption initialized")

	d, err := newDecompressor(r)
	if err != nil {
		log.Error("Could not initialise decompressor")

		return err

	}

	log.Debug("Decompression initialized")

	_, err = io.Copy(localTar, d)
	if err != nil {
		log.Errorf("Error in copying file")

		return err
	}

	log.Debug("Data copied")

	cmd := exec.Command("tar", "-xvf", "/home/backup.tar", "--directory", "/home/")
	var errMsg bytes.Buffer
	cmd.Stderr = &errMsg

	err = cmd.Run()
	if err != nil {
		return err
	}

	log.Debug("Untar file completed")

	err = d.Close()
	if err != nil {
		log.Errorf("Could not close decompressor: %v", err)
	}

	log.Info("Data copied succesfully")

	return nil
}

func (db DBConf) restore(sb s3Backend, keyPath, sqlDump string) error {
	log.Info("Start importing dump file")
	fr, err := sb.NewFileReader(sqlDump)
	if err != nil {
		return err
	}
	defer fr.Close()

	log.Debug("Read dump file")

	key := getKey(keyPath)
	r, err := newDecryptor(key, fr)
	if err != nil {
		log.Error("Could not initialise decryptor")

		return err
	}

	log.Debug("Decryption initialized")

	d, err := newDecompressor(r)
	if err != nil {
		log.Error("Could not initialise decompressor")

		return err

	}

	log.Debug("Decompression initialized")

	data, err := io.ReadAll(d)
	if err != nil {
		log.Error("Could not read all data")

		return err
	}
	d.Close()

	log.Debug("Data read successfully")

	dbURI := fmt.Sprintf("--dbname=postgresql://%s:%s@%s:%d/%s", db.user, db.password, db.host, db.port, db.database)
	cmd := exec.Command("pg_restore", dbURI)

	var in bytes.Buffer
	cmd.Stdin = &in
	in.Write(data)

	var errMsg bytes.Buffer
	cmd.Stderr = &errMsg

	err = cmd.Run()
	if err != nil {
		return err
	}

	log.Debug("Importing dump data finished")

	return nil
}

// buildConnInfo builds a connection string for the database
func buildConnInfo(db DBConf) string {
	dbURI := fmt.Sprintf("--dbname=postgresql://%s:%s@%s:%d/%s", db.user, db.password, db.host, db.port, db.database)

	var certsRequired bool

	log.Debugf("Postgres sslmode is set to: %v", db.sslMode)
	switch db.sslMode {
	case "allow":
		certsRequired = false
		dbURI += fmt.Sprintf("?sslmode=%s", db.sslMode)
	case "disable":
		certsRequired = false
		dbURI += fmt.Sprintf("?sslmode=%s", db.sslMode)
	case "prefer":
		certsRequired = false
	case "require":
		certsRequired = true
	case "verify-ca":
		certsRequired = true
	case "verify-full":
		certsRequired = true
	}

	if certsRequired {
		log.Debug("Certificates required for postgres connection")
		dbURI += fmt.Sprintf("?sslmode=%s", db.sslMode)

		if db.caCert != "" {
			dbURI += fmt.Sprintf("&sslrootcert=%s", db.caCert)
		}

		if db.clientCert != "" {
			dbURI += fmt.Sprintf("&sslcert=%s", db.clientCert)
		}

		if db.clientKey != "" {
			dbURI += fmt.Sprintf("&sslkey=%s", db.clientKey)
		}
	}

	return dbURI
}
