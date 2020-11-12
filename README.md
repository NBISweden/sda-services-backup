# Elasticsearch backups

## Create a key
Enable the transit encryption engine and create a key. Give it a descriptive name.

## Create some indices in ES
```sh
./main --action create --index index123
```

## Dumping encrypted index to S3
```sh
./main --action dump --index index123-mon-jan-8-17-43-24
s3cmd ls -c s3conf s3://dumps
s3cmd get -c s3conf s3://dumps/indexname
```

## Loading index from S3 to ES
```sh
./main --action load --index index123-mon-jan-8-17-43-24
```

## Example script configuration
```yaml
s3:
  url: "https://localhost"
  port: 9000
  accesskey: "myaccesskey"
  secretkey: "mysecretkey"
  bucket: "dumps"
  #chunksize: 32
  cacert: "./certs/ca.pem"
elastic:
  ## INSTANCE 1
  addr: "http://localhost:9200"
  ## INSTANCE 2
  #addr: "http://localhost:9201"
  user: "elastic"
  password: "elastic"
vault:
  addr: "http://localhost:8282"
  token: ""
  transitpath: "transit"
  key: "transit"
```