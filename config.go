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
	name   string
	action string
}

// Config is a parent object for all the different configuration parts
type Config struct {
	db             DBConf
	elastic        elasticConfig
	mongo          mongoConfig
	s3             S3Config
	publicKeyPath  string
	privateKeyPath string
	c4ghPassword   string
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

	flag.String("action", "backup", "action can be create, backup or restore")
	flag.String("name", "", "file name to create, backup or restore")

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	err := viper.BindPFlags(pflag.CommandLine)
	if err != nil {
		log.Fatalf("Could not bind process flags for commandline: %v", err)
	}

	action := viper.GetString("action")
	name := viper.GetString("name")

	return ClFlags{name: name, action: action}

}

// configS3Storage populates a S3Config
func configS3Storage(prefix string) S3Config {
	s3 := S3Config{}
	s3.URL = viper.GetString(prefix + ".url")
	s3.AccessKey = viper.GetString(prefix + ".accesskey")
	s3.SecretKey = viper.GetString(prefix + ".secretkey")
	s3.Bucket = viper.GetString(prefix + ".bucket")
	s3.Port = 443
	s3.Region = "us-east-1"

	if viper.IsSet(prefix + ".port") {
		s3.Port = viper.GetInt(prefix + ".port")
	}

	if viper.IsSet(prefix + ".region") {
		s3.Region = viper.GetString(prefix + ".region")
	}

	if viper.IsSet(prefix + ".chunksize") {
		s3.Chunksize = viper.GetInt(prefix+".chunksize") * 1024 * 1024
	}

	if viper.IsSet(prefix + ".cacert") {
		s3.Cacert = viper.GetString(prefix + ".cacert")
	}

	if viper.IsSet(prefix + ".PathPrefix") {
		s3.PathPrefix = viper.GetString(prefix + ".PathPrefix")
	}

	return s3
}

// configElastic populates a ElasticConfig
func configElastic() elasticConfig {
	elastic := elasticConfig{}
	elastic.host = viper.GetString("elastic.host")
	elastic.port = viper.GetInt("elastic.port")
	elastic.user = viper.GetString("elastic.user")
	elastic.password = viper.GetString("elastic.password")

	if viper.IsSet("elastic.batchSize") {
		elastic.batchSize = viper.GetInt("elastic.batchSize")
	}
	if viper.IsSet("elastic.filePrefix") {
		elastic.filePrefix = viper.GetString("elastic.filePrefix")
	}
	if viper.IsSet("elastic.cacert") {
		elastic.caCert = viper.GetString("elastic.cacert")
	}

	return elastic
}

// configPostgres populates a DBConf
func configPostgres() DBConf {
	pg := DBConf{}
	pg.host = viper.GetString("db.host")
	pg.port = 5432
	pg.user = viper.GetString("db.user")
	pg.password = viper.GetString("db.password")
	pg.database = viper.GetString("db.database")
	pg.sslMode = "prefer"

	if viper.IsSet("db.port") {
		pg.port = viper.GetInt("db.port")
	}

	if viper.IsSet("db.sslmode") {
		pg.sslMode = viper.GetString("db.sslmode")
		if pg.sslMode == "verify-full" {
			if !viper.IsSet("db.clientcert") || !viper.IsSet("db.clientkey") {
				log.Fatalln("client certificates are required when sslmode is 'verify-full'")
			}

			pg.clientCert = viper.GetString("db.clientcert")
			pg.clientKey = viper.GetString("db.clientkey")
		}
	}

	if viper.IsSet("db.cacert") {
		pg.caCert = viper.GetString("db.cacert")
	}

	return pg
}

// configMongoDB populates a MongoConfig
func configMongoDB() mongoConfig {
	mongo := mongoConfig{}
	mongo.host = viper.GetString("mongo.host")
	mongo.user = viper.GetString("mongo.user")
	mongo.password = viper.GetString("mongo.password")

	if viper.IsSet("mongo.authSource") {
		mongo.authSource = viper.GetString("mongo.authSource")
	}

	if viper.IsSet("mongo.port") {
		mongo.port = viper.GetInt("mongo.port")
	}

	if viper.IsSet("mongo.tls") {
		mongo.tls = viper.GetBool("mongo.tls")
		if viper.IsSet("mongo.cacert") {
			mongo.caCert = viper.GetString("mongo.cacert")
		}
		if viper.IsSet("mongo.clientcert") {
			mongo.clientCert = viper.GetString("mongo.clientcert")
			if mongo.clientCert == "" {
				log.Fatalln("client cert is required if TLS is true")
			}
		}
	}

	if viper.IsSet("mongo.replicaSet") {
		mongo.replicaSet = viper.GetString("mongo.replicaSet")
	}

	return mongo
}

func (c *Config) readConfig() {

	c.s3 = configS3Storage("s3")

	c.db = configPostgres()

	c.mongo = configMongoDB()

	c.elastic = configElastic()

	c.publicKeyPath = viper.GetString("crypt4ghPublicKey")

	c.privateKeyPath = viper.GetString("crypt4ghPrivateKey")

	c.c4ghPassword = viper.GetString("crypt4ghPassphrase")

	if viper.IsSet("loglevel") {
		stringLevel := viper.GetString("loglevel")
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
	if viper.IsSet("configPath") {
		cp := viper.GetString("conifgPath")
		ss := strings.Split(strings.TrimLeft(cp, "/"), "/")
		viper.AddConfigPath(path.Join(ss...))
	}
	if viper.IsSet("configFile") {
		viper.SetConfigFile(viper.GetString("configFile"))
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Infoln("No config file found, using ENVs only")
		} else {
			log.Fatalf("Error when reading config file: '%s'", err)
		}
	}
}
