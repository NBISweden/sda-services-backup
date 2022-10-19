#!/bin/bash

# Insert some test data in the db
docker exec db psql -U postgres -d test -c "INSERT INTO local_ega.main(submission_file_path,submission_user,submission_file_extension,status,encryption_method) VALUES('test.c4gh','dummy','c4gh','INIT','CRYPT4GH');"

# Build, run and execute the pg_basebackup action
docker container run --rm -i --name pg-backup --network=host $(docker build -f dev_tools/Dockerfile-backup -q -t backup .) /bin/sda-backup --action pg_basebackup

# Find the name of the copy in the S3 and check the length
DBCOPY=$(s3cmd -c dev_tools/s3conf ls s3://dumps/ | grep ".tar" | cut -d '/' -f4)
DBCOPYLENGTH=`echo -n "$DBCOPY" | wc -m`

# Find the name of the backup image created in the previous step and check the length
IMAGENAME=$(docker images -q backup)
NAMELENGTH=`echo -n "$IMAGENAME" | wc -m`

# Make sure that the image and the db copy exist before moving on
if [ $NAMELENGTH = 0 ] || [ $DBCOPYLENGTH = 0 ]; then
    echo "the image or the db copy is missing"
    exit 1
fi

# Get the db copy from S3
docker volume create restore
docker container run --rm -i --name pg-backup --network=host -v restore:/home backup /bin/sda-backup --action pg_db-unpack --name "$DBCOPY"

# Stop the running db
docker stop db

# Delete pgdata and add the physical copy in the db container
docker run --rm -v restore:/pg-backup -v dev_tools_pgData:/pg-data alpine /bin/sh -c "rm -r pg-data/pgdata && cp -r /pg-backup/db-backup/ /pg-data/pgdata/"

# start the DB container
docker start db

# Wait until the DB container is healthy
RETRY_TIMES=0
until [ $(docker inspect --format "{{json .State.Health.Status }}" db) = "\"healthy\"" ]
do
  echo "Waiting for container to become ready"
  RETRY_TIMES=$((RETRY_TIMES+1));
  echo $RETRY_TIMES
  if [ $RETRY_TIMES -eq 30 ]; then
    docker logs db | grep -i  "ERROR"
    exit 1;
  fi
  sleep 10;
done
echo "Finished waiting for container"

# Find the user in the db
USER=$(docker exec db psql -U postgres -d test -tA -c "select submission_user from local_ega.main where submission_file_path = 'test.c4gh';")

# Check if the user is the expected one
if [ "$USER" != "dummy" ]; then
    echo "Expected to get user 'dummy' but got '$USER'"
    exit 1
fi

# Remove the local folder
docker volume rm restore
