package main

import (
	"time"

	"github.com/cenkalti/backoff"
	"github.com/elastic/go-elasticsearch/v7"
	elastic "github.com/elastic/go-elasticsearch/v7"
	log "github.com/sirupsen/logrus"
)

func main() {

	flags := getCLflags()

	conf := NewConfig()
	log.Debug(conf.S3)

	sb, err := newS3Backend(conf.S3)

	if err != nil {
		log.Fatal(err)
	}

	retryBackoff := backoff.NewExponentialBackOff()

	tr := transportConfigES(conf.Elastic)

	c, err := elastic.NewClient(elasticsearch.Config{
		Addresses: []string{
			flags.instance,
		},
		Username:      conf.Elastic.user,
		Password:      conf.Elastic.password,
		RetryOnStatus: []int{502, 503, 504, 429},
		RetryBackoff: func(i int) time.Duration {
			if i == 1 {
				retryBackoff.Reset()
			}
			return retryBackoff.NextBackOff()
		},
		MaxRetries: 5,
		Transport:  tr,
	})

	if err != nil {
		log.Fatal(err)
	}

	switch flags.action {
	case "backup":
		log.Infof("Loading index %s into %s", flags.indexName, flags.instance)
		backupData(*sb, *c, conf.keyPath, flags.indexName, flags.batchsize)
	case "restore":
		err = countDocuments(*c, flags.indexName)
		if err != nil {
			log.Fatal(err)
		}
		log.Infof("Dumping index %s into %s", flags.indexName, flags.instance)
		restoreData(*sb, *c, conf.keyPath, flags.indexName, flags.batchsize)
	case "create":
		indexName := flags.indexName + "-" + "test"
		log.Infof("Creating index %s in %s", indexName, flags.instance)
		indexDocuments(*c, indexName)
	case "pg_dump":
		pgDump(*sb, conf.db, conf.keyPath)
	case "pg_restore":
		pgRestore(*sb, conf.db, conf.keyPath, flags.indexName)
	}
}

func restoreData(sb s3Backend, ec elastic.Client, keyPath, indexName string, batchsize int) {
	err := restoreDocuments(sb, ec, keyPath, indexName)
	if err != nil {
		log.Error(err)
	}
	log.Info("Done loading data from S3")

}

func backupData(sb s3Backend, ec elastic.Client, keyPath, indexName string, batchsize int) {
	err := backupDocuments(sb, ec, keyPath, indexName, batchsize)
	if err != nil {
		log.Error(err)
	}
	log.Info("Done dumping data to S3")
}
