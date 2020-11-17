package main

import (
	"flag"
	"path"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// ClFlags is an struc that holds cl flags info
type ClFlags struct {
	indexName string
	action    string
	instance  string
	batches   int
}

// Config is a parent object for all the different configuration parts
type Config struct {
	Elastic ElasticConfig
	S3      S3Config
	keyPath string
}

// NewConfig initializes and parses the config file and/or environment using
// the viper library.
func NewConfig() *Config {
	parseConfig()

	c := &Config{}
	c.readConfig()

	return c
}

// getCLflags returns the CL args of indexName and action
func getCLflags() ClFlags {

	flag.String("action", "create", "action can be create, dump or load")
	flag.Int("batches", 50, "number of batches to process the documents")
	flag.String("index", "index123", "index name to create, dump or load")
	flag.String("instance", "http://127.0.0.1:9200", "elasticsearch instance to perform the action")

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
    err := viper.BindPFlags(pflag.CommandLine)
    if err != nil {
        log.Fatalf("Could not bind process flags for commandline: %v", err)
    }

	action := viper.GetString("action")
	batches := viper.GetInt("batches")
	indexName := viper.GetString("index")
	instance := viper.GetString("instance")

	return ClFlags{indexName: indexName, action: action, instance: instance, batches: batches}

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

// configElastic populates a ElasticConfig
func configElastic() ElasticConfig {
	elastic := ElasticConfig{}
	elastic.user = viper.GetString("elastic.user")
	elastic.password = viper.GetString("elastic.password")
	elastic.caCert = viper.GetString("elastic.cacert")
	elastic.clientCert = viper.GetString("elastic.clientcert")
	elastic.clientKey = viper.GetString("elastic.clientkey")

	return elastic
}

func (c *Config) readConfig() {

	c.S3 = configS3Storage()

	c.Elastic = configElastic()

	c.keyPath = viper.GetString("key")

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
