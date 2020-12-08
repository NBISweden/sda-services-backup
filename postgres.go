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
