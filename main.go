package main

import (
	log "github.com/sirupsen/logrus"
)

func main() {

	flags := getCLflags()

	conf := NewConfig()
	pg := conf.db
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
		elastic.indexDocuments(flags.name)
	case "pg_dump":
		pg.dump(*sb, conf.keyPath)
	case "pg_restore":
		pg.restore(*sb, conf.keyPath, flags.name)
	}
}
