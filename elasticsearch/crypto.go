package main

import (
	"fmt"

	vault "github.com/mittwald/vaultgo"
	log "github.com/sirupsen/logrus"
)

// VaultConfig holds Vault settings
type VaultConfig struct {
	Addr             string
	Token            string
	TransitMountPath string
	Key              string
}

func encryptDocuments(c *vault.Client, document string, mountpath string, key string) string {
	const rsa4096 = "rsa-4096"

	fmt.Println(c.Token())

	transit := c.TransitWithMountPoint(mountpath)

	encryptResponse, err := transit.Encrypt(key, &vault.TransitEncryptOptions{
		Plaintext: document,
	})

	if err != nil {
		log.Fatalf("Error occurred during encryption: %v", err)
	}

	return encryptResponse.Data.Ciphertext
}

func decryptDocuments(c *vault.Client, encDocument string, mountpath string, key string) string {

	const rsa4096 = "rsa-4096"

	transit := c.TransitWithMountPoint(mountpath)

	decryptResponse, err := transit.Decrypt(key, &vault.TransitDecryptOptions{
		Ciphertext: encDocument,
	})
	if err != nil {
		log.Fatalf("Error occurred during decryption: %v", err)
	}

	return decryptResponse.Data.Plaintext
}
