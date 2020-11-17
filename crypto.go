package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

func getKey(key string) []byte {
	data, err := ioutil.ReadFile(key)
	if err != nil {
		log.Fatalf("Could not load cipher key: %s", err)
	}
	decodedkey, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		log.Fatalf("Could not decode base64 key: %s", err)
	}
	return decodedkey
}
func encryptDocs(hits gjson.Result, stream cipher.Stream, fr io.Writer) {
	var res strings.Builder
	fmt.Fprintf(&res, "%s\n", hits.Raw)
	plainText := []byte(res.String())
	cipherText := make([]byte, len(plainText))
	stream.XORKeyStream(cipherText, plainText)

	if _, err := io.Copy(fr, bytes.NewReader(cipherText)); err != nil {
		log.Fatal(err)
	}

}

func decryptDocs(rc io.Reader, key []byte) string {
	iv := make([]byte, aes.BlockSize)
	_, err := io.ReadFull(rc, iv)

	if err != nil {
		log.Fatalf("Reading iv from stream failed: %v", err)
	}

	stream := getStreamDecryptor(iv, key)

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(rc)
	data := buf.Bytes()
	if err != nil {
		log.Error(err)
	}

	// XORKeyStream can work in-place if the two arguments are the same.
	stream.XORKeyStream(data, data)

	out := string(data)
	return out
}

func getStreamEncryptor(key []byte) ([]byte, cipher.Stream) {
	block, err := aes.NewCipher(key)
	if err != nil {
		log.Fatal(err)
	}
	iv := make([]byte, aes.BlockSize)

	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		log.Fatal(err)
	}

	if err != nil {
		log.Fatal(err)
	}
	stream := cipher.NewCFBEncrypter(block, iv[:])

	return iv, stream
}

func getStreamDecryptor(iv, key []byte) cipher.Stream {
	block, err := aes.NewCipher(key)
	if err != nil {
		log.Fatal(err)
	}
	stream := cipher.NewCFBDecrypter(block, iv[:])
	return stream
}
