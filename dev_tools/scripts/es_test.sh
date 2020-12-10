#!/bin/bash

NOW=$(date '+%Y%m%d%H%M')

CONFIGFILE="dev_tools/config.yaml" go run . --action es_create --name "$NOW-123"
CONFIGFILE="dev_tools/config.yaml" go run . --action es_create --name "abc-$NOW"

CONFIGFILE="dev_tools/config.yaml" go run . --action es_backup --name "*$NOW*"

s3cmd ls -c dev_tools/s3conf s3://dumps/

CONFIGFILE="dev_tools/config.yaml" ELASTIC_PORT=9201 go run . --action es_restore --name "$NOW-123-test.bup"
CONFIGFILE="dev_tools/config.yaml" ELASTIC_PORT=9201 go run . --action es_restore --name "abc-$NOW-test.bup"

sleep 5

COUNT_1=$(curl http://127.0.0.1:9200/$NOW-test/_count | jq .count)
COUNT_2=$(curl http://127.0.0.1:9201/$NOW-test/_count | jq .count)

if [ "$COUNT_1" != "$COUNT_2" ]; then
  echo "Something went wrong, the imported backup does not contain all documents"
  echo "ES1 contains $COUNT_1 indices while ES2 contains $COUNT_2 indeices"
  exit 1
else
  echo "Backup was successful"
  exit 0
fi
