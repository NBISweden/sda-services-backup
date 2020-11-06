package main

import (
	"path"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// Config is a parent object for all the different configuration parts
type Config struct {
	Elastic ElasticConfig
	Vault   VaultConfig
	S3      S3Config
}

// NewConfig initializes and parses the config file and/or environment using
// the viper library.
func NewConfig() *Config {
	parseConfig()

	c := &Config{}
	c.readConfig()

	return c
}

// configS3Storage populates a S3Config
func configS3Storage() S3Config {
	s3 := S3Config{}
	s3.URL = viper.GetString("s3.url")
	s3.AccessKey = viper.GetString("s3.accesskey")
	s3.SecretKey = viper.GetString("s3.secretkey")
	s3.Bucket = viper.GetString("s3.bucket")
	s3.Port = 443
	s3.Region = "us-east-1"

	if viper.IsSet("s3.port") {
		s3.Port = viper.GetInt("s3.port")
	}

	if viper.IsSet("s3.region") {
		s3.Region = viper.GetString("s3.region")
	}

	if viper.IsSet("s3.chunksize") {
		s3.Chunksize = viper.GetInt("s3.chunksize") * 1024 * 1024
	}

	if viper.IsSet("s3.cacert") {
		s3.Cacert = viper.GetString("s3.cacert")
	}

	return s3
}

// configVault populates a VaultConfig
func configVault() VaultConfig {
	vault := VaultConfig{}
	vault.Addr = viper.GetString("vault.addr")
	vault.Token = viper.GetString("vault.token")
	vault.TransitMountPath = viper.GetString("vault.transitpath")
	vault.Key = viper.GetString("vault.key")

	return vault
}

// configElastic populates a ElasticConfig
func configElastic() ElasticConfig {
	elastic := ElasticConfig{}
	elastic.Addr = viper.GetString("elastic.addr")
	elastic.User = viper.GetString("elastic.user")
	elastic.Password = viper.GetString("elastic.password")
	elastic.Index = viper.GetString("elastic.index")

	return elastic
}

func (c *Config) readConfig() {

	c.S3 = configS3Storage()

	c.Elastic = configElastic()

	c.Vault = configVault()

	if viper.IsSet("log.level") {
		stringLevel := viper.GetString("log.level")
		intLevel, err := log.ParseLevel(stringLevel)
		if err != nil {
			log.Printf("Log level '%s' not supported, setting to 'trace'", stringLevel)
			intLevel = log.TraceLevel
		}
		log.SetLevel(intLevel)
		log.Printf("Setting log level to '%s'", stringLevel)
	}
}

func parseConfig() {
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetConfigType("yaml")
	if viper.IsSet("server.confPath") {
		cp := viper.GetString("server.confPath")
		ss := strings.Split(strings.TrimLeft(cp, "/"), "/")
		viper.AddConfigPath(path.Join(ss...))
	}
	if viper.IsSet("server.confFile") {
		viper.SetConfigFile(viper.GetString("server.confFile"))
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Infoln("No config file found, using ENVs only")
		} else {
			log.Fatalf("Error when reading config file: '%s'", err)
		}
	}
}
