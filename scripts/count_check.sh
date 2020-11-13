#!/bin/bash

COUNT_1=$(curl http://127.0.0.1:9201/index123-test/_count | jq .count)
COUNT_2=$(curl http://127.0.0.1:9201/index123-test/_count | jq .count)

echo "${COUNT_1}"
echo "${COUNT_2}"

if [ $COUNT_1 -ne 100 ] || [ $COUNT_2 -ne 100 ]; then
  echo "Something went wrong, the imported backup does not contain all documents"
  exit 1
else
  echo "Backup was successful"
  exit 0
fi