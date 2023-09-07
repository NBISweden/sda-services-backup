package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"os"

	"github.com/neicnordic/crypt4gh/keys"
	log "github.com/sirupsen/logrus"
)

// Generates a crypt4gh key pair, returning only the private key, as the
// public key used for encryption is the config file.
func generatePrivateKey() (*[32]byte, error) {
	log.Debug("Generating encryption key")

	_, privateKey, err := keys.GenerateKeyPair()
	if err != nil {
		return nil, err
	}

	return &privateKey, nil
}

func getKeys(path string) ([32]byte, [][32]byte) {
	// Generate private key
	privateKeyData, err := generatePrivateKey()
	if err != nil {
		log.Fatalf("Could not generate public key: %s", err)
	}

	// Get public key
	log.Debug("Getting public key")
	publicKey, err := os.Open(path)
	if err != nil {
		log.Fatalf("Could not open public key: %s", err)
	}
	publicKeyData, err := keys.ReadPublicKey(publicKey)
	if err != nil {
		log.Fatalf("Could not load public key: %s", err)
	}

	var publicKeyFileList [][32]byte
	publicKeyFileList = append(publicKeyFileList, publicKeyData)

	return *privateKeyData, publicKeyFileList
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
	stream := cipher.NewCFBDecrypter(block, iv)

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

	stream := cipher.NewCFBEncrypter(block, iv)

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
