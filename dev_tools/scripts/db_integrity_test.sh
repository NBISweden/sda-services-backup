#!/bin/bash

docker exec db psql -U postgres -d test -c "INSERT INTO local_ega.main(submission_file_path,submission_user,submission_file_extension,status,encryption_method) VALUES('test.c4gh','dummy','c4gh','INIT','CRYPT4GH');"

docker container run --rm -i --name pg-backup --network=host $(docker build -f dev_tools/Dockerfile-backup -q .) /bin/sda-backup --action pg_basebackup

DBCOPY=$(s3cmd -c dev_tools/s3conf ls s3://dumps/ | grep ".tar" | cut -d '/' -f4)

docker container run -v $(pwd)/tmp:/home --rm -i --name pg-backup --network=host $(docker build --build-arg USER_ID=$(id -u) -f dev_tools/Dockerfile-backup -q .) /bin/sda-backup --action pg_bb-restore --name "$DBCOPY"

docker exec -i db /bin/sh -c "rm -r data/pgdata"

docker cp tmp/db-backup/ db:data/pgdata

# Check when the container restarts
docker events --filter 'container=db'  | while read event
do
    check_event=$(echo $event | awk '{print $3}')
    if [ "$check_event" = "start" ]; then
        echo "container restarted"
        break
    fi
done;


USER=$(docker exec db psql -U postgres -d test -tA -c "select submission_user from local_ega.main where submission_file_path = 'test.c4gh';")

if [ "$USER" != "dummy" ]; then
    echo "Expected to get user 'dummy' but got '$USER'"
    exit 1
fi

rm -r tmp
