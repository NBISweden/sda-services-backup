package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/rand"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	elastic "github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esutil"
	vault "github.com/mittwald/vaultgo"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

// ElasticConfig is a Struct that holds ElasticSearch config
type ElasticConfig struct {
	Addr     string
	User     string
	Password string
	Index    string
}

func readResponse(r io.Reader) string {
	var b bytes.Buffer
	b.ReadFrom(r)
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

func getDocuments(sb s3Backend, es elastic.Client, vc vault.Client, indexName, keyName, mountpath string, batches int) error {

	var (
		batchNum int
		scrollID string
	)

	wr, err := sb.NewFileWriter(indexName + ".bup")
	key := getKey(&vc, mountpath, keyName)
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		log.Fatal(err)
	}
	stream := getStreamEncryptor([]byte(key), iv)

	log.Infoln("Scrolling through the documents...")

	es.Indices.Refresh(es.Indices.Refresh.WithIndex(indexName))

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
	wr.Write(iv)
	wr.Close()
	time.Sleep(time.Second * 8)
	return err
}

func bulkDocuments(sb s3Backend, c elastic.Client, vc vault.Client, indexName, keyName, mountpath string, batches int) error {
	var countSuccessful uint64
	ctx := context.Background()

	fr, err := sb.NewFileReader(indexName + ".bup")
	if err != nil {
		log.Error(err)
	}
	key := getKey(&vc, mountpath, keyName)
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
	bi.Close(ctx)
	fr.Close()
	time.Sleep(time.Second * 8)
	return err
}
