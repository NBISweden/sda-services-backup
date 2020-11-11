package main

import (
	"strconv"
	"strings"

	elastic "github.com/elastic/go-elasticsearch/v7"
	log "github.com/sirupsen/logrus"
)

func indexDocuments(es elastic.Client, indexName string) {
	log.Println("Indexing the documents...")
	for i := 1; i <= 100; i++ {
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
}
