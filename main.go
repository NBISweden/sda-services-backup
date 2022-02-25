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

	var elastic *esClient
	if conf.elastic != (elasticConfig{}) {
		elastic, err = newElasticClient(conf.elastic)
		if err != nil {
			log.Fatal(err)
		}
	}

	var mongo mongoConfig
	if conf.mongo != (mongoConfig{}) {
		mongo = conf.mongo
	}

	var pg DBConf
	if conf.db != (DBConf{}) {
		pg = conf.db
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
	case "mongo_dump":
		err := mongo.dump(*sb, conf.keyPath, flags.name)
		if err != nil {
			log.Fatal(err)
		}
	case "mongo_restore":
		err := mongo.restore(*sb, conf.keyPath, flags.name)
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
	case "pg_basebackup":
		err := pg.basebackup(*sb, conf.keyPath)
		if err != nil {
			log.Fatal(err)
		}
	}
}
