package main

import (
	"io"
	"os"
	"path/filepath"

	"github.com/neicnordic/crypt4gh/keys"
	"github.com/neicnordic/crypt4gh/streaming"
	log "github.com/sirupsen/logrus"
)

// Function for generating a crypt4gh private key
// which will be used for encrypting
func generatePrivateKey() (*[32]byte, error) {
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
func getKeys(path string) ([32]byte, [][32]byte, error) {
	privateKeyData, err := generatePrivateKey()
	if err != nil {
		log.Debug("Could not generate private key")

		return [32]byte{}, nil, err
	}

	path = filepath.Clean(path) // gosec G304
	publicKey, err := os.Open(path)
	if err != nil {
		log.Debug("Could not open public key")

		return [32]byte{}, nil, err
	}
	publicKeyData, err := keys.ReadPublicKey(publicKey)
	if err != nil {
		log.Debug("Could not load public key")

		return [32]byte{}, nil, err
	}

	var publicKeyFileList [][32]byte
	publicKeyFileList = append(publicKeyFileList, publicKeyData)

	return *privateKeyData, publicKeyFileList, nil
}

// Function for retrieving the private key (for decrypting) which is given in the config file
func getPrivateKey(path, password string) ([32]byte, error) {
	path = filepath.Clean(path) // gosec G304
	privateKey, err := os.Open(path)
	if err != nil {
		log.Debug("Could not open private key")

		return [32]byte{}, err
	}

	privateKeyData, err := keys.ReadPrivateKey(privateKey, []byte(password))
	if err != nil {
		log.Debug("Could not load private key")

		return [32]byte{}, err
	}

	return privateKeyData, nil
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
