# Elasticsearch backups

## Vault
Enable the transit encryption engine and create a key called `transit`.

## loading index from s3
```sh
./main index
```

## Dumping encrypted index to S3
```sh
./main dump
s3cmd ls -c s3conf s3://dumps
s3cmd get -c s3conf s3://dumps/indexname
```

## loading index from s3
```sh
./main load
```

## Script configuration
```yaml
s3:
  url: "https://localhost"
  port: 9000
  accesskey: "myaccesskey"
  secretkey: "mysecretkey!0"
  bucket: "dumps"
  cacert: "./certs/s3.pem"
elastic:
  #addr: "http://localhost:9200"
  addr: "http://localhost:9201"
  user: "elastic"
  password: "elastic"
  #index: "my-test-index"
  index: "my-test-index-mon-jan-6-11-26-58"
vault:
  addr: "http://localhost:8282"
  token: "s.lepNY18TQPM1QRZR9kW1DAhS"
  transitpath: "transit"
  key: "transit"
```