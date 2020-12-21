#!/bin/bash

NOW=$(date '+%Y%m%d%H%M')

for i in {1..100}; do
  name=$(head -c 10 /dev/urandom | base64)
  text=$(head -c 30 /dev/urandom | base64)
  curl -XPOST "http://127.0.0.1:9200/$NOW-123/_doc/$i" -d "{\"$name\": \"$text\" }" -H "Content-Type: application/json"
done

CONFIGFILE="dev_tools/config.yaml" go run . --action es_backup --name "*$NOW*"

if [ $? != 0 ]; then
  exit 1
fi

s3cmd ls -c dev_tools/s3conf s3://dumps/

CONFIGFILE="dev_tools/config.yaml" ELASTIC_PORT=9201 go run . --action es_restore --name "$NOW-123.bup"

sleep 5

COUNT_1=$(curl http://127.0.0.1:9200/$NOW-123/_count | jq .count)
COUNT_2=$(curl http://127.0.0.1:9201/$NOW-123/_count | jq .count)

if [ "$COUNT_1" != "$COUNT_2" ] || [ "$COUNT_1" != 100 ] || [ "$COUNT_2" != 100 ]; then
  echo "Something went wrong, the imported backup does not contain all documents"
  echo "ES1 contains $COUNT_1 indices while ES2 contains $COUNT_2 indeices"
  exit 1
else
  echo "Backup was successful"
  exit 0
fi
