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
	var iname string
	inarg := os.Args[1]

	if inarg == "" || (inarg != "load" && inarg != "dump" && inarg != "index") {
		log.Fatal("Failed to start script. You need to provide [dump/load/index i_name] ")
	}
	if inarg == "index" {
		iname = os.Args[2]
		if iname == "" {
			log.Fatal("Failed to start script. You need to provide an index name")
		}
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
		loadData(*sb, *c, *vc, conf.Elastic.Index, conf.Vault.TransitMountPath, conf.Vault.Key)
	} else if inarg == "dump" {
		dumpData(*sb, *c, *vc, conf.Elastic.Index, conf.Vault.TransitMountPath, conf.Vault.Key)
	} else if inarg == "index" {
		indexName := iname + "-" + time.Now().Format("mon-jan-2-15-04-05")
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

	encryptIndex(&vc, documents.String(), mountPath, keyName)
	wr, err := sb.NewFileWriter("thisisatest")

	if err != nil {
		log.Info(err)
	}
	wr.Write([]byte("asd"))
	wr.Close()

	if err != nil {
		log.Info(err)
	}

	log.Info("Done dumping data to S3")
}
