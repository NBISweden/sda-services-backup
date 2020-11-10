package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	vault "github.com/mittwald/vaultgo"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

// VaultConfig holds Vault settings
type VaultConfig struct {
	Addr             string
	Token            string
	TransitMountPath string
	Key              string
}

func getKey(c *vault.Client, mountpath string, key string) string {

	transit := c.TransitWithMountPoint(mountpath)

	res, err := transit.Read(key)
	if err != nil {
		log.Fatal(err)
	} else {
		log.Printf("%+v\n", res.Data)
	}

	exportRes, err := transit.Export(key, vault.TransitExportOptions{
		KeyType: "encryption-key",
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("all keys : %v+", exportRes.Data.Keys)
	log.Printf("%v+", exportRes.Data.Keys[1])

	decodedKey, err := base64.StdEncoding.DecodeString(exportRes.Data.Keys[1])
	if err != nil {
		log.Fatal(err)
	}

	return string(decodedKey)
}

func encryptDocs(hits gjson.Result, stream cipher.Stream, fr io.Writer) {
	log.Info(hits.Raw)
	var res strings.Builder
	fmt.Fprintf(&res, "%s\n", hits.Raw)
	plainText := []byte(res.String())
	cipherText := make([]byte, len(plainText))
	stream.XORKeyStream(cipherText, plainText)

	if _, err := io.Copy(fr, bytes.NewReader(cipherText)); err != nil {
		log.Fatal(err)
	}

}

func decryptDocs(rc io.ReadCloser, key []byte) string {
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(rc)
	data := buf.Bytes()
	log.Info(data)
	if err != nil {
		log.Error(err)
	}

	stream := getStreamDecryptor(key)

	// XORKeyStream can work in-place if the two arguments are the same.
	stream.XORKeyStream(data, data)

	out := string(data)
	return out
}

func getStreamEncryptor(key []byte) cipher.Stream {
	block, err := aes.NewCipher(key)
	if err != nil {
		log.Fatal(err)
	}
	var iv [aes.BlockSize]byte
	if _, err := io.ReadFull(rand.Reader, iv[:]); err != nil {
		panic(err)
	}
	if err != nil {
		log.Fatal(err)
	}
	stream := cipher.NewCFBEncrypter(block, iv[:])

	return stream
}

func getStreamDecryptor(key []byte) cipher.Stream {
	block, err := aes.NewCipher(key)
	if err != nil {
		log.Fatal(err)
	}
	var iv [aes.BlockSize]byte
	stream := cipher.NewCFBDecrypter(block, iv[:])
	return stream
}
