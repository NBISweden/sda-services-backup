package main

import (
	"io"
	"os"

	"github.com/neicnordic/crypt4gh/keys"
	"github.com/neicnordic/crypt4gh/streaming"
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

// Function for getting the public key which is given in the config file
// and the private key which is generated on the fly and not stored.
// Returns the generated private key and a list with the public key
// in order to encrypt the file.
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

// Function for getting the private key (for decrypting) which is given in the config file
func getPrivateKey(path, password string) [32]byte {
	// Get private key
	log.Debug("Getting private key")
	privateKey, err := os.Open(path)
	if err != nil {
		log.Fatalf("Could not open private key: %s", err)
	}

	privateKeyData, err := keys.ReadPrivateKey(privateKey, []byte(password))
	if err != nil {
		log.Fatalf("Could not load private key: %s", err)
	}

	return privateKeyData
}

func newDecryptor(privateKey [32]byte, r io.Reader) (*streaming.Crypt4GHReader, error) {
	crypt4GHReader, err := streaming.NewCrypt4GHReader(r, privateKey, nil)
	if err != nil {
		return nil, err
	}

	return crypt4GHReader, nil
}

func newEncryptor(pubKeyList [][32]byte, privateKey [32]byte, w io.Writer) (*streaming.Crypt4GHWriter, error) {

	crypt4GHWriter, err := streaming.NewCrypt4GHWriter(w, privateKey, pubKeyList, nil)
	if err != nil {
		return nil, err
	}

	return crypt4GHWriter, nil
}
