package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esutil"
	"github.com/tidwall/gjson"

	log "github.com/sirupsen/logrus"
)

// ElasticConfig is a Struct that holds ElasticSearch config
type elasticConfig struct {
	host       string
	port       int
	user       string
	password   string
	caCert     string
	batchSize  int
	filePrefix string
}

type esClient struct {
	client *elasticsearch.Client
	conf   elasticConfig
}

func newElasticClient(config elasticConfig) (*esClient, error) {
	retryBackoff := backoff.NewExponentialBackOff()

	tr := transportConfigES(config)
	URI := esURI(config)
	c, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{
			URI,
		},
		Username:      config.user,
		Password:      config.password,
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

	return &esClient{client: c, conf: config}, err
}

// transportConfigES is a helper method to setup TLS for the ES client.
func transportConfigES(config elasticConfig) http.RoundTripper {
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
		cacert, e := os.ReadFile(config.caCert)
		if e != nil {
			log.Fatalf("failed to append %q to RootCAs: %v", cacert, e)
		}
		if ok := cfg.RootCAs.AppendCertsFromPEM(cacert); !ok {
			log.Debug("no certs appended, using system certs only")
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

func (es esClient) countDocuments(indexName string) error {

	log.Infoln("Couting documents to fetch...")
	log.Infoln(strings.Repeat("-", 80))
	cr, err := es.client.Count(es.client.Count.WithIndex(indexName))

	if err != nil {
		log.Error(err)
	}

	json := readResponse(cr.Body)
	cr.Body.Close()

	count := int(gjson.Get(json, "count").Int())

	log.Infof("Found %v documents", count)

	return err
}

func findIndices(es esClient, indexGlob string) ([]string, error) {

	log.Infoln("Finding indices to fetch...")
	log.Infoln(strings.Repeat("-", 80))

	catObj := es.client.Cat.Indices
	cr, err := es.client.Cat.Indices(catObj.WithIndex(indexGlob), catObj.WithFormat("JSON"), catObj.WithH("index"))

	if err != nil {
		log.Error(err)
	}

	json := readResponse(cr.Body)
	cr.Body.Close()

	result := gjson.Get(json, "#.index")

	var indices []string
	for _, index := range result.Array() {
		indices = append(indices, index.String())
	}

	log.Debugf("Found indices: %v", indices)

	return indices, err

}

func (es esClient) backupDocuments(sb *s3Backend, keyPath, indexGlob string) error {
	log.Infof("Backing up indexes that match glob: %s", indexGlob)
	var (
		batchNum int
		scrollID string
	)

	batchsize := 50
	filePrefix := ""

	if es.conf.batchSize != 0 {
		batchsize = es.conf.batchSize
	}

	if es.conf.filePrefix != "" {
		filePrefix = es.conf.filePrefix
	}

	targetIndices, err := findIndices(es, indexGlob)

	if err != nil {
		log.Error("Could not find indices to fetch")

		return err
	}

	for _, index := range targetIndices {
		wg := sync.WaitGroup{}
		wr, err := sb.NewFileWriter(filePrefix+index+".bup", &wg)

		if err != nil {
			log.Fatalf("Could not open backup file for writing: %v", err)
		}

		key := getKey(keyPath)

		e, err := newEncryptor(key, wr)

		if err != nil {
			log.Error("Could not initialize encryptor")

			return err
		}

		c, err := newCompressor(e)

		if err != nil {
			log.Error("Could not initialize encryptor")

			return err
		}

		_, err = es.client.Indices.Refresh(es.client.Indices.Refresh.WithIndex(index))

		if err != nil {
			log.Error("Could not refresh indexes")

			return err
		}

		res, err := es.client.Search(
			es.client.Search.WithIndex(index),
			es.client.Search.WithSize(batchsize),
			es.client.Search.WithSort("_doc"),
			es.client.Search.WithScroll(time.Second*60),
		)

		if err != nil {
			return err
		}

		json := readResponse(res.Body)
		res.Body.Close()

		hits := gjson.Get(json, "hits.hits")
		_, err = c.Write([]byte(hits.Raw + "\n"))
		if err != nil {
			log.Error("Could not encrypt/write")

			return err
		}

		log.Debug("Batch   ", batchNum)
		log.Trace("ScrollID", scrollID)
		log.Trace("IDs     ", gjson.Get(hits.Raw, "#._id"))
		log.Trace(strings.Repeat("-", 80))

		scrollID = gjson.Get(json, "_scroll_id").String()

		for {
			batchNum++

			res, err := es.client.Scroll(es.client.Scroll.WithScrollID(scrollID), es.client.Scroll.WithScroll(time.Minute))
			if err != nil {
				return err
			}
			if res.IsError() {
				log.Error("Error response")

				return err
			}

			json = readResponse(res.Body)
			res.Body.Close()

			scrollID = gjson.Get(json, "_scroll_id").String()

			hits := gjson.Get(json, "hits.hits")
			log.Trace(hits)

			if len(hits.Array()) < 1 {
				log.Traceln("Finished scrolling")

				break
			}

			_, err = c.Write([]byte(hits.Raw + "\n"))
			if err != nil {
				log.Error("Could not encrypt/write")

				return err
			}
			log.Debug("Batch   ", batchNum)
			log.Trace("ScrollID", scrollID)
			log.Trace("IDs     ", gjson.Get(hits.Raw, "#._id"))
			log.Trace(strings.Repeat("-", 80))
		}
		c.Close()
		wr.Close()
		wg.Wait()
	}

	return nil
}

func (es *esClient) restoreDocuments(sb *s3Backend, keyPath, fileName string) error {
	var countSuccessful uint64

	err := es.countDocuments(fileName)
	if err != nil {
		return err
	}

	log.Infof("restoring index with name %s", fileName)

	fr, err := sb.NewFileReader(fileName)
	if err != nil {
		return err
	}
	defer fr.Close()

	key := getKey(keyPath)
	r, err := newDecryptor(key, fr)
	if err != nil {
		log.Error("Could not initialise decryptor")

		return err
	}
	d, err := newDecompressor(r)
	if err != nil {
		log.Error("Could not initialise decompressor")

		return err

	}
	data, err := io.ReadAll(d)
	if err != nil {
		log.Error("Could not read all data")

		return err
	}
	d.Close()

	ud := string(data)

	indexName := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	bi, err := esutil.NewBulkIndexer(esutil.BulkIndexerConfig{
		Index:         indexName,
		Client:        es.client,
		NumWorkers:    1,
		FlushBytes:    int(2048),
		FlushInterval: 30 * time.Second,
	})
	if err != nil {
		log.Error("Unexpected error")

		return err
	}
	defer bi.Close(context.Background())

	for _, docs := range strings.Split(ud, "\n") {
		if docs == "" {
			log.Debug("End of blob reached")

			break
		}
		i := 0
		for {
			key := fmt.Sprintf("%v._source", i)
			source := gjson.Get(docs, key).String()

			if source == "" {
				break
			}

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
				log.Error("Unexpected error")

				return err
			}
			i++
		}
	}

	return nil
}

func esURI(c elasticConfig) string {
	URI := c.host

	URI = fmt.Sprintf(URI+":%d", c.port)

	return URI
}
