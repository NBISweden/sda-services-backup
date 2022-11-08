package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"

	log "github.com/sirupsen/logrus"
)

func getKey(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Could not load cipher key: %s", err)
	}
	decodedkey, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		log.Fatalf("Could not decode base64 key: %s", err)
	}

	return decodedkey
}

type encryptor struct {
	stream cipher.Stream
	w      io.Writer
}

type decryptor struct {
	stream cipher.Stream
	r      io.Reader
}

func newDecryptor(key []byte, r io.Reader) (*decryptor, error) {
	iv := make([]byte, aes.BlockSize)
	_, err := io.ReadFull(r, iv)

	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCFBDecrypter(block, iv[:])

	return &decryptor{
		stream: stream,
		r:      r,
	}, nil
}

func (d *decryptor) Read(p []byte) (n int, err error) {
	b := make([]byte, len(p))
	n, err = d.r.Read(b)
	if n == 0 {
		return n, err
	}
	d.stream.XORKeyStream(p, b)

	return n, err
}

func newEncryptor(key []byte, w io.Writer) (*encryptor, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	iv := make([]byte, aes.BlockSize)

	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	stream := cipher.NewCFBEncrypter(block, iv[:])

	l, err := w.Write(iv)

	if err != nil {
		return nil, err
	}
	if l != len(iv) {
		return nil, fmt.Errorf("Ecnryptor, failed to write iv")
	}

	return &encryptor{
		stream: stream,
		w:      w,
	}, nil
}

func (e *encryptor) Write(p []byte) (n int, err error) {
	b := make([]byte, len(p))
	e.stream.XORKeyStream(b, p)
	n, err = e.w.Write(b)

	return
}
