# Elasticsearch backups

## Create a key

## Create some indices in ES

```cmd
./main --action create --index index123
```

## Backing up encrypted index to S3

```cmd
./main --action backup --index index123-test
s3cmd ls -c s3conf s3://dumps
s3cmd get -c s3conf s3://dumps/index123-test.bup
```

## Restoring index from S3 to ES

```cmd
./main --action restore --index index123-test --instance http://127.0.0.1:9201
```

## Backing up a database

```cmd
./main --action pg_dump
```

## Restoring up a database

The target database must exist when restoring the data.

```cmd
./main --action pg_restore --index <pg-dump-file> 
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
db:
  host: "localhost"
  user: "postgres"
  password: "postgres"
  database: "test"
```
