package main

import (
	log "github.com/sirupsen/logrus"
)

func main() {

	flags := getCLflags()

	conf := NewConfig()
	log.Debug(conf.s3)

	sb, err := newS3Backend(conf.s3)
	if err != nil {
		log.Fatal(err)
	}

	elastic, err := newElasticClient(conf.elastic)
	if err != nil {
		log.Fatal(err)
	}

	switch flags.action {
	case "es_backup":
		elastic.backupDocuments(sb, conf.keyPath, flags.name)
	case "es_restore":
		elastic.restoreDocuments(sb, conf.keyPath, flags.name)
	case "es_create":
		indexName := flags.name + "-" + "test"
		log.Infof("Creating index %s", indexName)
		elastic.indexDocuments(indexName)
	case "pg_dump":
		pgDump(*sb, conf.db, conf.keyPath)
	case "pg_restore":
		pgRestore(*sb, conf.db, conf.keyPath, flags.name)
	}
}
