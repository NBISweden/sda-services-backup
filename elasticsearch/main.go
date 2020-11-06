package main

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/elastic/go-elasticsearch/v7"
	elastic "github.com/elastic/go-elasticsearch/v7"
	vault "github.com/mittwald/vaultgo"
	log "github.com/sirupsen/logrus"
)

func main() {

	inarg := os.Args[1]

	if inarg == "" || (inarg != "load" && inarg != "dump") {
		log.Fatal("Failed to start script. You need to provide [dump/load] ")
	}

	conf := NewConfig()
	log.Info(conf.S3)
	sb, err := newS3Backend(conf.S3)

	if err != nil {
		log.Error(err)
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

	if inarg == "load" {
		log.Infof("Loading index %s into ES", conf.Elastic.Index)
		loadData(*sb, *c, *vc, conf.Elastic.Index)
	} else if inarg == "dump" {
		indexName := conf.Elastic.Index + "-" + time.Now().Format("mon-jan-2-15-04-05")
		log.Infof("Dumping index %s into ES", indexName)
		dumpData(*sb, *c, *vc, indexName)
	}
}

func loadData(sb s3Backend, ec elastic.Client, vc vault.Client, indexName string) {
	batches := 5
	fr, err := sb.NewFileReader(indexName + ".bup")

	if err != nil {
		log.Error("Unable to read from S3")
		return
	}

	buf := new(strings.Builder)
	_, err = io.Copy(buf, fr)
	plaintext := descryptIndex(&vc, buf.String(), "transit", "transit")

	bulkDocuments(ec, indexName, plaintext, batches)
	log.Info("Done loading data from S3")

}
func dumpData(sb s3Backend, ec elastic.Client, vc vault.Client, indexName string) {
	batches := 5
	indexDocuments(ec, indexName)

	time.Sleep(time.Second * 10)

	documents, err := getDocuments(ec, indexName, batches)

	if err != nil {
		log.Error(err)
	}

	encIndex := encryptIndex(&vc, documents.String(), "transit", "transit")
	wr, err := sb.NewFileWriter(indexName + ".bup")

	if err != nil {
		log.Info(err)
	}
	wr.Write([]byte(encIndex))
	wr.Close()

	if err != nil {
		log.Info(err)
	}

	log.Info("Done dumping data to S3")
}
