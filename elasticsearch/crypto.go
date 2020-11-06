package main

import (
	"fmt"

	vault "github.com/mittwald/vaultgo"
	log "github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v3"
)

// VaultConfig holds Vault settings
type VaultConfig struct {
	Addr             string
	Token            string
	TransitMountPath string
	Key              string
}

func encryptIndex(c *vault.Client, index string, mountpath string, key string) string {
	const rsa4096 = "rsa-4096"

	fmt.Println(c.Token())

	transit := c.TransitWithMountPoint(mountpath)

	err := transit.Create(key, &vault.TransitCreateOptions{
		Exportable: null.BoolFrom(true),
		Type:       rsa4096,
	})
	if err != nil {
		log.Fatal(err)
	}

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
	log.Printf("%v+", exportRes.Data.Keys[1])

	encryptResponse, err := transit.Encrypt(key, &vault.TransitEncryptOptions{
		Plaintext: index,
	})

	if err != nil {
		log.Fatalf("Error occurred during encryption: %v", err)
	}

	return encryptResponse.Data.Ciphertext
}

func descryptIndex(c *vault.Client, encIndex string, mountpath string, key string) string {

	const rsa4096 = "rsa-4096"

	transit := c.TransitWithMountPoint(mountpath)

	decryptResponse, err := transit.Decrypt(key, &vault.TransitDecryptOptions{
		Ciphertext: encIndex,
	})
	if err != nil {
		log.Fatalf("Error occurred during decryption: %v", err)
	}

	return decryptResponse.Data.Plaintext
}
