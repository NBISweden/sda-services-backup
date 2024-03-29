#!/bin/bash

num=100

if [ -n "$1" ]; then
  num=$1
fi

NOW=$(date '+%Y%m%d%H%M')

for i in $(seq 1 "$num"); do
  name=$(head -c 10 /dev/urandom | base64)
  text=$(head -c 30 /dev/urandom | base64)
  curl -XPOST "http://127.0.0.1:9200/$NOW-123/_doc/$i" -d "{\"$name\": \"$text\" }" -H "Content-Type: application/json"
done

# bail early since if this fails the rest will fail also
if ! CONFIGFILE="dev_tools/config_elastic.yaml" go run . --action es_backup --name "*$NOW*"; then
  exit 1
fi

# check that we fail on non existing index
echo "checking for non existing index"
if CONFIGFILE="dev_tools/config_elastic.yaml" go run . --action es_backup --name "*Foo*"; then
  echo "Failure was expected here"
  exit 1
fi

s3cmd ls -c dev_tools/s3conf s3://dumps/

echo "restoring index"
CONFIGFILE="dev_tools/config_elastic.yaml" ELASTIC_PORT=9201 go run . --action es_restore --name "$NOW-123.bup"

sleep 5

COUNT_1=$(curl "http://127.0.0.1:9200/$NOW-123/_count" | jq .count)
COUNT_2=$(curl "http://127.0.0.1:9201/$NOW-123/_count" | jq .count)

if [ "$COUNT_1" != "$COUNT_2" ] || [ "$COUNT_1" != "$num" ] || [ "$COUNT_2" != "$num" ]; then
  echo "Something went wrong, the imported backup does not contain all documents"
  echo "ES1 contains $COUNT_1 indices while ES2 contains $COUNT_2 indeices"
  exit 1
else
  echo "Backup was successful"
  exit 0
fi
