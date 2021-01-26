#!/bin/bash
RETRY_TIMES=0
for p in elastic elastic2 mys3 db mongodb-0 mongodb-1
do
  until [ $(docker inspect --format "{{json .State.Health.Status }}" $p) = "\"healthy\"" ]
  do
    echo "Waiting for $p to become ready"
    docker logs $p | grep -i  "ERROR"
    RETRY_TIMES=$((RETRY_TIMES+1));
    echo $RETRY_TIMES
    if [ $RETRY_TIMES -eq 30 ]; then
      exit 1;
    fi
    sleep 10;
  done
  echo "Finished waiting for $p"
done
