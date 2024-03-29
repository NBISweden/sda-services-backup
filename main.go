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
		log.Fatal("Could not connect to s3 backend: ", err)
	}

	log.Infof("Connection to s3 bucket %v established", sb.Bucket)

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
		err := elastic.backupDocuments(sb, conf.publicKeyPath, flags.name)
		if err != nil {
			log.Fatal(err)
		}
	case "es_restore":
		err := elastic.restoreDocuments(sb, conf.privateKeyPath, flags.name, conf.c4ghPassword)
		if err != nil {
			log.Fatal(err)
		}
	case "mongo_dump":
		err := mongo.dump(*sb, conf.publicKeyPath, flags.name)
		if err != nil {
			log.Fatal(err)
		}
	case "mongo_restore":
		err := mongo.restore(*sb, conf.privateKeyPath, flags.name, conf.c4ghPassword)
		if err != nil {
			log.Fatal(err)
		}
	case "pg_dump":
		err := pg.dump(*sb, conf.publicKeyPath)
		if err != nil {
			log.Fatal(err)
		}
	case "pg_restore":
		err := pg.restore(*sb, conf.privateKeyPath, flags.name, conf.c4ghPassword)
		if err != nil {
			log.Fatal(err)
		}
	case "pg_basebackup":
		err := pg.basebackup(*sb, conf.publicKeyPath)
		if err != nil {
			log.Fatal(err)
		}
	case "pg_db-unpack":
		err := pg.baseBackupUnpack(*sb, conf.privateKeyPath, flags.name, conf.c4ghPassword)
		if err != nil {
			log.Fatal(err)
		}
	}
}
