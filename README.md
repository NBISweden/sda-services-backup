# Elasticsearch backups

## Create a key


## Create some indices in ES
```sh
./main --action create --index index123
```

## Dumping encrypted index to S3
```sh
./main --action dump --index index123-test
s3cmd ls -c s3conf s3://dumps
s3cmd get -c s3conf s3://dumps/index123-test.bup
```

## Loading index from S3 to ES
```sh
./main --action load --index index123-test --instance http://127.0.0.1:9201
```

## Example script configuration
```yaml
s3:
  url: "https://127.0.0.1"
  port: 9000
  accesskey: "myaccesskey"
  secretkey: "mysecretkey"
  bucket: "dumps"
  #chunksize: 32
  cacert: "./certs/ca.pem"
elastic:
  user: "elastic"
  password: "elastic"
```
