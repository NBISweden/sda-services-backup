package main

import (
	"time"

	"github.com/cenkalti/backoff"
	"github.com/elastic/go-elasticsearch/v7"
	elastic "github.com/elastic/go-elasticsearch/v7"
	vault "github.com/mittwald/vaultgo"
	log "github.com/sirupsen/logrus"
)

func main() {

	indexName, action := getCLflags()

	conf := NewConfig()
	log.Debug(conf.S3)

	sb, err := newS3Backend(conf.S3)

	if err != nil {
		log.Fatal(err)
	}

	vcfg := VaultConfig{Addr: conf.Vault.Addr, Token: conf.Vault.Token}
	vc, err := vault.NewClient(vcfg.Addr, vault.WithCaPath(""), vault.WithAuthToken(vcfg.Token))

	retryBackoff := backoff.NewExponentialBackOff()

	ecfg := ElasticConfig{Addr: conf.Elastic.Addr, User: conf.Elastic.User, Password: conf.Elastic.Password}

	c, err := elastic.NewClient(elasticsearch.Config{
		Addresses: []string{
			ecfg.Addr,
		},
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

	switch action {
	case "load":
		log.Infof("Loading index %s into %s", indexName, conf.Elastic.Addr)
		loadData(*sb, *c, *vc, indexName, conf.Vault.TransitMountPath, conf.Vault.Key)
	case "dump":
		countDocuments(*c, indexName)
		log.Infof("Dumping index %s into %s", indexName, conf.Elastic.Addr)
		dumpData(*sb, *c, *vc, indexName, conf.Vault.TransitMountPath, conf.Vault.Key)
	case "create":
		indexName := indexName + "-" + time.Now().Format("mon-jan-2-15-04-05")
		log.Infof("Creating index %s in %s", indexName, conf.Elastic.Addr)
		indexDocuments(*c, indexName)
	}
}

func loadData(sb s3Backend, ec elastic.Client, vc vault.Client, indexName string, mountPath string, keyName string) {
	batches := 5
	err := bulkDocuments(sb, ec, vc, indexName, keyName, mountPath, batches)
	if err != nil {
		log.Error(err)
	}
	log.Info("Done loading data from S3")

}
func dumpData(sb s3Backend, ec elastic.Client, vc vault.Client, indexName string, mountPath string, keyName string) {
	batches := 5
	err := getDocuments(sb, ec, vc, indexName, keyName, mountPath, batches)
	if err != nil {
		log.Error(err)
	}
	log.Info("Done dumping data to S3")
}
