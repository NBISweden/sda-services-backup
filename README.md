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

#### Dump

* backup will be stored in S3 in the format of `YYYYMMDDhhmmss-DBNAME.sqldump`

```cmd
./backup-svc --action pg_dump
```

#### Pg_basebackup

* backup will be stored in S3 in the format of `YYYYMMDDhhmmss-DBNAME.tar`

```cmd
docker container run --rm -i --name pg-backup --network=host $(docker build -f dev_tools/Dockerfile-backup -q -t backup .) /bin/sda-backup --action pg_basebackup
```

**NOTE**

This type of backup runs through a docker container because of some compatibility issues
that might appear between the PostgreSQL 13 running in the `db` container and the local one.

### Restoring up a database

#### Restore dump file

* The target database must exist when restoring the data.

```cmd
./backup-svc --action pg_restore --name PG-DUMP-FILE
```

#### Restore from physical copy

This is done in more stages.

* The target database must be stopped before restoring it.

* Create a docker volume for the physical copy.

* Get the physical copy from the S3 and unpack it in the docker volume which was created in the previous step
```cmd
docker container run --rm -i --name pg-backup --network=host -v <docker-volume>:/home $(docker build -f dev_tools/Dockerfile-backup -q -t backup .) /bin/sda-backup --action pg_db-unpack --name TAR-FILE
```

* Copy the backup from the its docker volume to the pgdata of the database's docker volume
```cmd
docker run --rm -v <docker-volume>:/pg-backup -v <database-docker-volume>:/pg-data alpine cp -r /pg-backup/db-backup/ /pg-data/<target-pgdata>/
```

* Start the database container.

**NOTE**

Again here a docker container is used for the same reason explained in the `Pg_basebackup` section.

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
  host: "FQDN URI" # https://es.example.com
  #port: 9200 # only needed if the port difers from the standard HTTP/HTTPS ports
  user: "elastic-user"
  password: "elastic-password"
  #cacert: "path/to/ca-root"
  batchSize: 50 # How many documents to retrieve from elastic search at a time, default 50 (should probably be at least 2000
  filePrefix: "" # Can be emtpy string, useful in case an index has been written to and you want to backup a new copy
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
