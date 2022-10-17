package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
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
	today := time.Now().Format("20060102150405")
	destDir := "db-backup"
	dbURI := buildConnInfo(db)
	cmd := exec.Command("pg_basebackup", dbURI, "-F", "p", "-D", destDir)

	var errMsg bytes.Buffer
	cmd.Stderr = &errMsg

	err := cmd.Run()
	if err != nil {
		log.Errorf(errMsg.String())
		return err
	}

	cmd = exec.Command("pg_verifybackup", destDir)

	cmd.Stderr = &errMsg

	err = cmd.Run()
	if err != nil {
		log.Errorf(errMsg.String())
		return err
	}

	cmd = exec.Command("tar", "-cvf", destDir+".tar", destDir)

	cmd.Stderr = &errMsg

	err = cmd.Run()
	if err != nil {
		log.Errorf(errMsg.String())
		return err
	}

	fileName := today + "-" + db.database + ".tar"
	wg := sync.WaitGroup{}
	wr, err := sb.NewFileWriter(fileName, &wg)
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

	c, err := newCompressor(e)
	if err != nil {
		log.Errorf("Could not initialize compressor: (%v)", err)
		return err
	}

	sourceFileName := destDir + ".tar"
	data, err := ioutil.ReadFile(sourceFileName)
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

	err = wr.Close()
	if err != nil {
		log.Errorf("Could not close destination file: %v", err)
	}
	wg.Wait()

	return nil
}

func (db DBConf) dump(sb s3Backend, keyPath string) error {
	today := time.Now().Format("20060102150405")
	dbURI := buildConnInfo(db)
	cmd := exec.Command("pg_dump", dbURI, "-xF", "tar")

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
	wr, err := sb.NewFileWriter(today+"-"+db.database+".sqldump", &wg)
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

	c, err := newCompressor(e)
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

// BasebackupUnpack function:
// - gets the key to decrypt the pg_data
// - decrypts and decompress the data
// - untar the data
// - puts the db copy in the running container
func (db DBConf) baseBackupUnpack(sb s3Backend, keyPath, backupTar string) error {
	localTar, err := os.Create("/home/backup.tar")
	if err != nil {
		log.Errorf("Error in creating file: %v", err)
	}

	fr, err := sb.NewFileReader(backupTar)
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

	d, err := newDecompressor(r)
	if err != nil {
		log.Errorf("Could not initialise decompressor: %v", err)
		return err

	}

	_, err = io.Copy(localTar, d)
	if err != nil {
		log.Errorf("Error in copying file: %v", err)
		return err
	}

	cmd := exec.Command("tar", "-xvf", "/home/backup.tar", "--directory", "/home/")
	var errMsg bytes.Buffer
	cmd.Stderr = &errMsg

	err = cmd.Run()
	if err != nil {
		log.Errorf(errMsg.String())
		return err
	}

	err = d.Close()
	if err != nil {
		log.Errorf("Could not close decompressor: %v", err)
	}

	return nil
}

func (db DBConf) restore(sb s3Backend, keyPath, sqlDump string) error {

	fr, err := sb.NewFileReader(sqlDump)
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
	d, err := newDecompressor(r)
	if err != nil {
		log.Error("Could not initialise decompressor", err)
		return err

	}
	data, err := ioutil.ReadAll(d)
	if err != nil {
		log.Error("Could not read all data: ", err)
		return err
	}
	d.Close()

	dbURI := fmt.Sprintf("--dbname=postgresql://%s:%s@%s:%d/%s", db.user, db.password, db.host, db.port, db.database)
	cmd := exec.Command("pg_restore", dbURI)

	var in bytes.Buffer
	cmd.Stdin = &in
	in.Write(data)

	var errMsg bytes.Buffer
	cmd.Stderr = &errMsg

	err = cmd.Run()
	if err != nil {
		log.Errorf(errMsg.String())
		return err
	}

	return nil
}

// buildConnInfo builds a connection string for the database
func buildConnInfo(db DBConf) string {
	dbURI := fmt.Sprintf("--dbname=postgresql://%s:%s@%s:%d/%s", db.user, db.password, db.host, db.port, db.database)

	var certsRequired bool

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
	case "verify-peer":
		certsRequired = true
	}

	if certsRequired {
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
