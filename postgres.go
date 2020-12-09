package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
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

func pgDump(sb s3Backend, db DBConf, keyPath string) {
	today := time.Now().Format("20060102150405")
	dbURI := fmt.Sprintf("--dbname=postgresql://%s:%s@%s:%d/%s", db.user, db.password, db.host, db.port, db.database)
	cmd := exec.Command("pg_dump", dbURI, "-xF", "tar")

	var out bytes.Buffer
	cmd.Stdout = &out

	var errMsg bytes.Buffer
	cmd.Stderr = &errMsg

	err := cmd.Run()
	if err != nil {
		log.Fatalf(errMsg.String())
	}

	wg := sync.WaitGroup{}
	wr, err := sb.NewFileWriter(today+"-"+db.database+".sqldump", &wg)
	if err != nil {
		log.Fatalf("Could not open backup file for writing: %v", err)
	}

	key := getKey(keyPath)
	e, err := NewEncryptor(key, wr)
	if err != nil {
		log.Fatalf("Could not initialize encryptor: (%v)", err)
	}

	c, err := newCompressor(key, e)
	if err != nil {
		log.Fatalf("Could not initialize compressor: (%v)", err)
	}

	_, err = c.Write(out.Bytes())
	if err != nil {
		log.Fatalf("Could not encrypt/write: %s", err)
	}

	c.Close()
	wr.Close()
	wg.Wait()
}

func pgRestore(sb s3Backend, db DBConf, keyPath, sqlDump string) {

	fr, err := sb.NewFileReader(sqlDump)
	if err != nil {
		log.Error(err)
	}
	defer fr.Close()

	key := getKey(keyPath)
	r, err := NewDecryptor(key, fr)
	if err != nil {
		log.Error("Could not initialise decryptor", err)
	}
	d, err := newDecompressor(key, r)
	if err != nil {
		log.Error("Could not initialise decompressor", err)

	}
	data, err := ioutil.ReadAll(d)
	if err != nil {
		log.Error("Could not read all data: ", err)
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
		log.Fatalf(errMsg.String())
	}
}
