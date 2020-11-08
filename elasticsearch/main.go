package main

import (
	"io"
	"strings"
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

	switch action {
	case "load":
		log.Infof("Loading index %s into ES", indexName)
		loadData(*sb, *c, *vc, indexName, conf.Vault.TransitMountPath, conf.Vault.Key)
	case "dump":
		dumpData(*sb, *c, *vc, indexName, conf.Vault.TransitMountPath, conf.Vault.Key)
	case "create":
		indexName := indexName + "-" + time.Now().Format("mon-jan-2-15-04-05")
		log.Infof("Dumping index %s into ES", indexName)
		indexDocuments(*c, indexName)
	}
}

func loadData(sb s3Backend, ec elastic.Client, vc vault.Client, indexName string, mountPath string, keyName string) {
	batches := 5
	fr, err := sb.NewFileReader(indexName + ".bup")

	if err != nil {
		log.Error("Unable to read from S3")
		return
	}

	buf := new(strings.Builder)
	_, err = io.Copy(buf, fr)
	plaintext := decryptIndex(&vc, buf.String(), mountPath, keyName)

	bulkDocuments(ec, indexName, plaintext, batches)
	log.Info("Done loading data from S3")

}
func dumpData(sb s3Backend, ec elastic.Client, vc vault.Client, indexName string, mountPath string, keyName string) {
	batches := 5
	documents, err := getDocuments(ec, indexName, batches)

	if err != nil {
		log.Error(err)
	}

	encIndex := encryptIndex(&vc, documents.String(), mountPath, keyName)
	wr, err := sb.NewFileWriter(indexName + ".bup")

	if err != nil {
		log.Info(err)
	}

	_, err = wr.Write([]byte(encIndex))
	if err != nil {
		log.Info(err)
	}

	time.Sleep(time.Second * 10)
	if err != nil {
		log.Info(err)
	}
	wr.Close()
	time.Sleep(time.Second * 5)

	log.Info("Done dumping data to S3")
}
