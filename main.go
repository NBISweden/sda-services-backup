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
		err := elastic.backupDocuments(sb, conf.keyPath, flags.name)
		if err != nil {
			log.Fatal(err)
		}
	case "es_restore":
		err := elastic.restoreDocuments(sb, conf.keyPath, flags.name)
		if err != nil {
			log.Fatal(err)
		}
	case "pg_dump":
		err := pg.dump(*sb, conf.keyPath)
		if err != nil {
			log.Fatal(err)
		}
	case "pg_restore":
		err := pg.restore(*sb, conf.keyPath, flags.name)
		if err != nil {
			log.Fatal(err)
		}
	}
}
