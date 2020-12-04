package main

import (
	"bytes"
	"compress/zlib"
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

func findIndices(es elastic.Client, indexGlob string) ([]string, error) {

	log.Infoln("Finding indices to fetch...")
	log.Infoln(strings.Repeat("-", 80))

	catObj := es.Cat.Indices
	cr, err := es.Cat.Indices(catObj.WithIndex(indexGlob), catObj.WithFormat("JSON"), catObj.WithH("index"))

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

func newCompressor(key []byte, w io.Writer) (io.WriteCloser, error) {

	zw := zlib.NewWriter(w)
	_, err := zlib.NewWriterLevel(zw, zlib.BestCompression)

	if err != nil {
		log.Error("Unable to set zlib writer level", err)
	}
	return zw, nil
}

func newDecompressor(key []byte, r io.Reader) (io.ReadCloser, error) {

	zr, err := zlib.NewReader(r)
	if err != nil {
		log.Error("Unable to create zlib reader", err)
	}
	return zr, err
}

func backupDocuments(sb s3Backend, es elastic.Client, keyPath, indexGlob string, batchsize int) error {

	var (
		batchNum int
		scrollID string
	)

	targetIndices, err := findIndices(es, indexGlob)

	if err != nil {
		log.Fatalf("Could not find indices to fetch: %v", err)
	}

	for _, index := range targetIndices {
		wg := sync.WaitGroup{}
		wr, err := sb.NewFileWriter(index+".bup", &wg)

		if err != nil {
			log.Fatalf("Could not open backup file for writing: %v", err)
		}

		key := getKey(keyPath)

		e, err := NewEncryptor(key, wr)

		if err != nil {
			log.Fatalf("Could not initialize encryptor: (%v)", err)
		}

		c, err := newCompressor(key, e)

		if err != nil {
			log.Fatalf("Could not initialize encryptor: (%v)", err)
		}

		_, err = es.Indices.Refresh(es.Indices.Refresh.WithIndex(index))

		if err != nil {
			log.Fatalf("Could not refresh indexes: %v", err)
		}

		res, err := es.Search(
			es.Search.WithIndex(index),
			es.Search.WithSize(batchsize),
			es.Search.WithSort("_doc"),
			es.Search.WithScroll(time.Second*60),
		)

		if err != nil {
			log.Error(err)
		}

		json := readResponse(res.Body)
		res.Body.Close()

		hits := gjson.Get(json, "hits.hits")
		_, err = c.Write([]byte(hits.Raw + "\n"))
		if err != nil {
			log.Fatalf("Could not encrypt/write: %s", err)
		}

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
				_, err = c.Write([]byte(hits.Raw + "\n"))
				if err != nil {
					log.Fatalf("Could not encrypt/write: %s", err)
				}
				log.Info("Batch   ", batchNum)
				log.Debug("ScrollID", scrollID)
				log.Debug("IDs     ", gjson.Get(hits.Raw, "#._id"))
				log.Debug(strings.Repeat("-", 80))
			}
		}
		c.Close()
		wr.Close()
		wg.Wait()
	}

	return err
}

func restoreDocuments(sb s3Backend, c elastic.Client, keyPath, indexName string) error {
	var countSuccessful uint64

	fr, err := sb.NewFileReader(indexName + ".bup")
	if err != nil {
		log.Error(err)
	}
	defer fr.Close()

	key := getKey(keyPath)
	r, err := NewDecryptor(key, fr)
	if err != nil {
		log.Error("Could not initialise decryptor", err)
	}
	d, err := newDecompressor(key, r)
	if err != nil {
		log.Error("Could not initialise decompressor", err)

	}
	data, err := ioutil.ReadAll(d)
	if err != nil {
		log.Error("Could not read all data: ", err)
	}
	d.Close()

	if err != nil {
		log.Error(err)
	}
	ud := string(data)

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
				log.Fatalf("Unexpected error: %s", err)
			}
			i++
		}
	}

	return err
}
