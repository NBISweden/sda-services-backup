package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	elastic "github.com/elastic/go-elasticsearch/v7"
	log "github.com/sirupsen/logrus"
)

// GenerateRandomBytes generates a rnd string
func generateRandomBytes(n int) string {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		log.Fatal(err)
	}
	rb := base64.StdEncoding.EncodeToString(b)

	return rb
}

func indexDocuments(es elastic.Client, indexName string) {
	log.Println("Indexing the documents...")
	for i := 1; i <= 100; i++ {
		str := fmt.Sprintf(`{"%s" : "%s"}`, generateRandomBytes(20), generateRandomBytes(10))
		res, err := es.Index(
			indexName,
			strings.NewReader(str),
			es.Index.WithDocumentID(strconv.Itoa(i)),
		)

		if err != nil || res.IsError() {
			log.Error("Error happens here")
			log.Fatalf("Error: %s: %s", err, res)
		}
		time.Sleep(time.Millisecond * 50)
	}
}
