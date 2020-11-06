package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	elastic "github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esutil"
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

func indexDocuments(es elastic.Client, indexName string) {
	log.Println("Indexing the documents...")
	for i := 1; i <= 2000; i++ {
		res, err := es.Index(
			indexName,
			strings.NewReader(`{"title" : "test"}`),
			es.Index.WithDocumentID(strconv.Itoa(i)),
		)

		if err != nil || res.IsError() {
			log.Error("Error happens here")
			log.Fatalf("Error: %s: %s", err, res)
		}
	}
	time.Sleep(time.Second * 4)
}

func checkDocuments(es elastic.Client, indexName string) {
	res, err := es.Search(
		es.Search.WithIndex(indexName),
		es.Search.WithSize(10),
		es.Search.WithSort("_doc"),
	)

	if err != nil {
		log.Error(err)
	}

	json := readResponse(res.Body)
	res.Body.Close()

	hits := gjson.Get(json, "hits.hits")
	log.Info(hits)

}

func getDocuments(es elastic.Client, indexName string, batches int) (strings.Builder, error) {

	var (
		batchNum int
		scrollID string
	)

	log.Println("Couting documents to fetch...")
	log.Println(strings.Repeat("-", 80))
	cr, err := es.Count(es.Count.WithIndex(indexName))

	if err != nil {
		log.Error(err)
	}

	json := readResponse(cr.Body)
	cr.Body.Close()

	count := int(gjson.Get(json, "count").Int())

	log.Printf("Found %v documents", count)

	es.Indices.Refresh(es.Indices.Refresh.WithIndex(indexName))

	log.Println("Scrolling...")
	log.Println(strings.Repeat("-", 80))

	res, err := es.Search(
		es.Search.WithIndex(indexName),
		es.Search.WithSize(batches),
		es.Search.WithSort("_doc"),
		es.Search.WithScroll(time.Second*60),
	)

	if err != nil {
		log.Error(err)
	}

	var results strings.Builder

	json = readResponse(res.Body)
	res.Body.Close()

	hits := gjson.Get(json, "hits.hits")

	fmt.Fprintf(&results, "%s\n", hits.Raw)

	log.Println("Batch   ", batchNum)
	log.Println("ScrollID", scrollID)
	log.Println("IDs     ", gjson.Get(hits.Raw, "#._id"))
	log.Println(strings.Repeat("-", 80))

	scrollID = gjson.Get(json, "_scroll_id").String()

	// Perform the scroll requests in sequence
	//
	for {
		batchNum++

		// Perform the scroll request and pass the scrollID and scroll duration
		//
		res, err := es.Scroll(es.Scroll.WithScrollID(scrollID), es.Scroll.WithScroll(time.Minute))
		if err != nil {
			log.Fatalf("Error: %s", err)
		}
		if res.IsError() {
			log.Fatalf("Error response: %s", res)
		}

		json = readResponse(res.Body)
		res.Body.Close()

		// Extract the scrollID from response
		//
		scrollID = gjson.Get(json, "_scroll_id").String()

		// Extract the search results
		//
		hits := gjson.Get(json, "hits.hits")
		log.Info(hits)

		// Break out of the loop when there are no results
		//
		if len(hits.Array()) < 1 {
			log.Println("Finished scrolling")
			break
		} else {
			fmt.Fprintf(&results, "%s\n", hits.Raw)

			log.Println("Batch   ", batchNum)
			log.Println("ScrollID", scrollID)
			log.Println("IDs     ", gjson.Get(hits.Raw, "#._id"))
			log.Println(strings.Repeat("-", 80))
		}
	}
	return results, err
}

func bulkDocuments(c elastic.Client, indexName, documents string, batches int) error {
	var countSuccessful uint64

	bi, err := esutil.NewBulkIndexer(esutil.BulkIndexerConfig{
		Index:         indexName,
		Client:        &c,
		NumWorkers:    1,
		FlushBytes:    int(2048),
		FlushInterval: 30 * time.Second,
	})

	for _, docs := range strings.Split(documents, "\n") {
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
