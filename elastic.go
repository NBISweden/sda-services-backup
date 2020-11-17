package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	elastic "github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esutil"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

// ElasticConfig is a Struct that holds ElasticSearch config
type ElasticConfig struct {
	user       string
	password   string
	verifyPeer bool
	caCert     string
	clientCert string
	clientKey  string
}

// transportConfigES is a helper method to setup TLS for the ES client.
func transportConfigES(config ElasticConfig) http.RoundTripper {
	cfg := new(tls.Config)

	// Enforce TLS1.2 or higher
	cfg.MinVersion = 2

	// Read system CAs
	var systemCAs, _ = x509.SystemCertPool()
	if reflect.DeepEqual(systemCAs, x509.NewCertPool()) {
		log.Debug("creating new CApool")
		systemCAs = x509.NewCertPool()
	}
	cfg.RootCAs = systemCAs

	if config.caCert != "" {
		cacert, e := ioutil.ReadFile(config.caCert)
		if e != nil {
			log.Fatalf("failed to append %q to RootCAs: %v", cacert, e)
		}
		if ok := cfg.RootCAs.AppendCertsFromPEM(cacert); !ok {
			log.Debug("no certs appended, using system certs only")
		}
	}

	if config.verifyPeer {
		if config.clientCert == "" || config.clientKey == "" {
			log.Fatalf("No client cert or key were provided")
		}

		cert, e := ioutil.ReadFile(config.clientCert)
		if e != nil {
			log.Fatalf("failed to append client cert %q: %v", config.clientCert, e)
		}
		key, e := ioutil.ReadFile(config.clientKey)
		if e != nil {
			log.Fatalf("failed to append key %q: %v", config.clientKey, e)
		}
		if certs, e := tls.X509KeyPair(cert, key); e == nil {
			cfg.Certificates = append(cfg.Certificates, certs)
		}
	}

	var trConfig http.RoundTripper = &http.Transport{
		TLSClientConfig:   cfg,
		ForceAttemptHTTP2: true}

	return trConfig
}

func readResponse(r io.Reader) string {
	var b bytes.Buffer
    _, err := b.ReadFrom(r)
    if err != nil { // Maybe propagate this error upwards?
        log.Fatal(err)
    }
	return b.String()
}

func countDocuments(es elastic.Client, indexName string) error {

	log.Infoln("Couting documents to fetch...")
	log.Infoln(strings.Repeat("-", 80))
	cr, err := es.Count(es.Count.WithIndex(indexName))

	if err != nil {
		log.Error(err)
	}

	json := readResponse(cr.Body)
	cr.Body.Close()

	count := int(gjson.Get(json, "count").Int())

	log.Infof("Found %v documents", count)

	return err
}

func getDocuments(sb s3Backend, es elastic.Client, keyPath, indexName string, batches int) error {

	var (
		batchNum int
		scrollID string
	)

	wg := sync.WaitGroup{}

	wr, err := sb.NewFileWriter(indexName+".bup", &wg)
    if err != nil {
        log.Fatalf("Could not open backup file for writing: %v", err)
    }

	key := getKey(keyPath)
	iv, stream := getStreamEncryptor([]byte(key))
	l, err := wr.Write(iv)

	if l != len(iv) || err != nil {
		log.Fatalf("Could not write all of iv (%d vs %d) or write failed (%v)", l, len(iv), err)
	}

	log.Infoln("Scrolling through the documents...")

	_, err = es.Indices.Refresh(es.Indices.Refresh.WithIndex(indexName))

    if err != nil {
        log.Fatalf("Could not refresh indexes: %v", err)
    }

	res, err := es.Search(
		es.Search.WithIndex(indexName),
		es.Search.WithSize(batches),
		es.Search.WithSort("_doc"),
		es.Search.WithScroll(time.Second*60),
	)

	if err != nil {
		log.Error(err)
	}

	json := readResponse(res.Body)

	hits := gjson.Get(json, "hits.hits")
	encryptDocs(hits, stream, wr)

	log.Info("Batch   ", batchNum)
	log.Debug("ScrollID", scrollID)
	log.Debug("IDs     ", gjson.Get(hits.Raw, "#._id"))
	log.Debug(strings.Repeat("-", 80))

	scrollID = gjson.Get(json, "_scroll_id").String()

	for {
		batchNum++

		res, err := es.Scroll(es.Scroll.WithScrollID(scrollID), es.Scroll.WithScroll(time.Minute))
		if err != nil {
			log.Fatalf("Error: %s", err)
		}
		if res.IsError() {
			log.Fatalf("Error response: %s", res)
		}

		json = readResponse(res.Body)
		res.Body.Close()

		scrollID = gjson.Get(json, "_scroll_id").String()

		hits := gjson.Get(json, "hits.hits")
		log.Debug(hits)

		if len(hits.Array()) < 1 {
			log.Infoln("Finished scrolling")
			break
		} else {
			encryptDocs(hits, stream, wr)
			log.Info("Batch   ", batchNum)
			log.Debug("ScrollID", scrollID)
			log.Debug("IDs     ", gjson.Get(hits.Raw, "#._id"))
			log.Debug(strings.Repeat("-", 80))
		}
	}
	wr.Close()
	wg.Wait()
	return err
}

func bulkDocuments(sb s3Backend, c elastic.Client, keyPath, indexName string, batches int) error {
	var countSuccessful uint64

	fr, err := sb.NewFileReader(indexName + ".bup")
	if err != nil {
		log.Error(err)
	}
	defer fr.Close()

	key := getKey(keyPath)
	ud := decryptDocs(fr, []byte(key))

	bi, err := esutil.NewBulkIndexer(esutil.BulkIndexerConfig{
		Index:         indexName,
		Client:        &c,
		NumWorkers:    1,
		FlushBytes:    int(2048),
		FlushInterval: 30 * time.Second,
	})
	if err != nil {
		log.Fatalf("Unexpected error: %s", err)
	}
	defer bi.Close(context.Background())

	for _, docs := range strings.Split(ud, "\n") {
		if docs == "" {
			log.Info("End of blob reached")
			break
		}
		for i := 0; i < batches; i++ {
			key := fmt.Sprintf("%v._source", i)
			source := gjson.Get(docs, key).String()

			err = bi.Add(
				context.Background(),
				esutil.BulkIndexerItem{
					Action: "index",
					Body:   strings.NewReader(source),
					OnSuccess: func(ctx context.Context, item esutil.BulkIndexerItem, res esutil.BulkIndexerResponseItem) {
						atomic.AddUint64(&countSuccessful, 1)
					},
					OnFailure: func(ctx context.Context, item esutil.BulkIndexerItem, res esutil.BulkIndexerResponseItem, err error) {
						if err != nil {
							log.Errorf("Error: %s", err)
						} else {
							log.Errorf("Error: %s: %s", res.Error.Type, res.Error.Reason)
						}
					},
				},
			)
			if err != nil {
				log.Fatalf("Unexpected error: %s", err)
			}
		}
	}

	return err
}
