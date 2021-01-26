# Elasticsearch backups

## Build the app

```cmd
go build -ldflags "-extldflags -static" -o backup-svc .
```

## Configuration

The specific config file to be used can be set via the environmental variable `CONFIGFILE`,
which holds the full path to the config file.

All parts of the config file can be set as ENVs where the separator is `_` i.e. the S3 accesskey can be set as `S3_ACCESSKEY`.  
ENVs will overrule values set in the config file

For a complete example of configuration options see the [example](#Example-configuration-file) at the bottom

## Create a key

The encryption key should be in `aes-256-gcm` format and needs to be passed as as bas64 encoded string in a file

## Elasticsearch

### Backing up encrypted index to S3

```cmd
./backup-svc --action es_backup --name [ can be a glob `*INDEX-NAME*` ]
```

* backup will be stored in S3 in the format of FULL-ES-INDEX-NAME.bup

Verify that the backup worked:

```cmd
s3cmd -c PATH_TO_S3CONF_FILE ls s3://BUCKET-NAME/*INDEX-NAME
```

### Restoring index from S3 to ES

```cmd
./backup-svc --action es_restore --name S3-OBJECT-NAME
```

## Create some indices in ES (only for teting)

```cmd
./backup-svc --action es_create --name INDEX-NAME
```

## Postgres backup

### Backing up a database

* backup will be stored in S3 in the format of `YYYYMMDDhhmmss-DBNAME.sqldump`

```cmd
./backup-svc --action pg_dump
```

### Restoring up a database

* The target database must exist when restoring the data.

```cmd
./backup-svc --action pg_restore --name PG-DUMP-FILE
```

## MongoDB

### Backing up a database

* backup will be stored in S3 in the format of `YYYYMMDDhhmmss-DBNAME.archive`

```cmd
./backup-svc --action mongo_dump --name <DBNAME>
```

### Restoring up a database


```cmd
./backup-svc --action mongo_restore --name MONGO-ARCHIVE-FILE
```


## Example configuration file

```yaml
encryptionKey: "aes256.key"
loglevel: debug
s3:
  url: "FQDN URI" #https://s3.example.com
  #port: 9000 #only needed if the port difers from the standard HTTP/HTTPS ports
  accesskey: "accesskey"
  secretkey: "secret-accesskey"
  bucket: "bucket-name"
  #cacert: "path/to/ca-root"
elastic:
  host: "FQDN URI" #https://es.example.com
  #port: 9200 #only neede if the port difers from the standard HTTP/HTTPS ports
  user: "elastic-user"
  password: "elastic-password"
  #cacert: "path/to/ca-root"
db:
  host: "hostname or IP" #pg.example.com, 127.0.0.1
  #port: 5432 #only needed if the postgresql databse listens to a different port
  user: "db-user"
  password: "db-password"
  database: "database-name"
  #cacert: "path/to/ca-root"
  #clientcert: "path/to/clientcert" #only needed if sslmode = verify-peer
  #clientkey: "path/to/clientkey" #only needed if sslmode = verify-peer
  #sslmode: "verify-peer" #
mongo:
  host: "hostname or IP with portnuber" #example.com:portnumber, 127.0.0.1:27017
  user: "backup"
  password: "backup"
  authSource: "admin"
  replicaset: ""
  #tls: true
  #cacert: "path/to/ca-root" #optional
  #clientcert: "path/to/clientcert" # needed if tls=true
```
