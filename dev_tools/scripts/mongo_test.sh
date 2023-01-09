#!/bin/bash

num=100
docker=false

if [ -n "$1" ] && [ "$1" -eq "$1" ]; then
  num=$1
elif [ -n "$1" ] && [ "$1" == "$1" ]; then
  docker=$1
fi

if [ -n "$2" ] && [ "$2" -eq "$2" ]; then
  num=$2
elif [ -n "$2" ] && [ "$2" == "$2" ]; then
  docker=$2
fi

NOW=$(date '+%Y%m%d%H%M')
TLS="--tls --tlsCAFile=/certs/ca.pem --tlsCertificateKeyFile=/certs/server.pem"

for i in $(seq 1 $num); do
  name=$(head -c 10 /dev/urandom | base64)
  text=$(head -c 30 /dev/urandom | base64)
  docker exec mongodb-0 mongo -u root -p password123 --host localhost:27017 $TLS --eval 'db.getSiblingDB("'$NOW'").data.insert({"'$name'": "'$text'"})' --quiet
done

# create user with only "backup" and "restore" permissions
docker exec mongodb-0 mongo admin -u root -p password123 --host localhost:27017 $TLS --eval 'db.createUser({user: "backup", pwd: "backup", roles: [{role: "backup", db:"admin"},{role: "restore",db: "admin"}]})' || true

if [ "$docker" == "docker" ]; then
  docker run --rm --network=dev_tools_default -v $PWD/dev_tools:/conf/:ro -e CONFIGFILE="/conf/dockerfile_config_mongo.yaml" nbisweden/sda-backup:test backup-svc --action mongo_dump --name $NOW
else
  CONFIGFILE="dev_tools/config_mongo.yaml" go run . --action mongo_dump --name "$NOW"
fi

# bail early since if this fails the rest will fail also
if [ $? != 0 ]; then
  echo "bailing on error"
  exit 1
fi

COUNT_1=$(docker exec mongodb-0 mongo -u root -p password123 --authenticationDatabase=admin --host localhost:27017 $TLS --eval 'db.getSiblingDB("'$NOW'").stats().objects' --quiet | tr -d "\r")

s3cmd ls -c dev_tools/s3conf s3://dumps/

docker exec mongodb-0 mongo -u root -p password123 --host localhost:27017 $TLS --eval 'db.getSiblingDB("'$NOW'").dropDatabase()' --quiet

DUMPFILE=$(s3cmd -c dev_tools/s3conf ls s3://dumps/ | grep "$NOW.archive" | cut -d '/' -f4)
echo "restoring databse from file $DUMPFILE"

if [ "$docker" == "docker" ]; then
  docker run --rm --network=dev_tools_default -v $PWD/dev_tools:/conf/:ro -e CONFIGFILE="/conf/dockerfile_config_mongo.yaml" nbisweden/sda-backup:test backup-svc --action mongo_restore --name "$DUMPFILE"
else
  CONFIGFILE="dev_tools/config_mongo.yaml" go run . --action mongo_restore --name "$DUMPFILE"
fi
sleep 5

COUNT_2=$(docker exec mongodb-0 mongo -u root -p password123 --host localhost:27017 $TLS --eval 'db.getSiblingDB("'$NOW'").stats().objects' --quiet | tr -d "\r")

if [ "$COUNT_1" != "$COUNT_2" ] || [ "$COUNT_1" != "$num" ] || [ "$COUNT_2" != "$num" ]; then
  echo "Something went wrong, the imported backup does not contain all documents"
  echo "Source contained $COUNT_1 indices, while restored contains $COUNT_2 indices"
  exit 1
else
  echo "Backup was successful"
  exit 0
fi
