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

	c, err := elastic.NewClient(elasticsearch.Config{
		Addresses: []string{
			flags.instance,
		},
		Username:      conf.Elastic.User,
		Password:      conf.Elastic.Password,
		RetryOnStatus: []int{502, 503, 504, 429},
		RetryBackoff: func(i int) time.Duration {
			if i == 1 {
				retryBackoff.Reset()
			}
			return retryBackoff.NextBackOff()
		},
		MaxRetries: 5,
	})

	if err != nil {
		log.Fatal(err)
	}

	switch flags.action {
	case "load":
		log.Infof("Loading index %s into %s", flags.indexName, flags.instance)
		loadData(*sb, *c, conf.keyPath, flags.indexName)
	case "dump":
		countDocuments(*c, flags.indexName)
		log.Infof("Dumping index %s into %s", flags.indexName, flags.instance)
		dumpData(*sb, *c, conf.keyPath, flags.indexName)
	case "create":
		indexName := flags.indexName + "-" + "test"
		log.Infof("Creating index %s in %s", indexName, flags.instance)
		indexDocuments(*c, indexName)
	}
}

func loadData(sb s3Backend, ec elastic.Client, keyPath, indexName string) {
	batches := 5
	err := bulkDocuments(sb, ec, keyPath, indexName, batches)
	if err != nil {
		log.Error(err)
	}
	log.Info("Done loading data from S3")

}
func dumpData(sb s3Backend, ec elastic.Client, keyPath, indexName string) {
	batches := 5
	err := getDocuments(sb, ec, keyPath, indexName, batches)
	if err != nil {
		log.Error(err)
	}
	log.Info("Done dumping data to S3")
}
