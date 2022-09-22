#!/bin/bash

# Insert some test data in the db
docker exec db psql -U postgres -d test -c "INSERT INTO local_ega.main(submission_file_path,submission_user,submission_file_extension,status,encryption_method) VALUES('test.c4gh','dummy','c4gh','INIT','CRYPT4GH');"

# Build, run and execute the pg_basebackup action
docker container run --rm -i --name pg-backup --network=host $(docker build --build-arg USER_ID=$(id -u) -f dev_tools/Dockerfile-backup -q -t backup .) /bin/sda-backup --action pg_basebackup

# Find the name of the copy in the S3
DBCOPY=$(s3cmd -c dev_tools/s3conf ls s3://dumps/ | grep ".tar" | cut -d '/' -f4)

# Checks if the backup image exists locally from the previous step, if not creates the image
# Then gets the physical copy of the database from the S3
IMAGENAME=$(docker images -q backup)
NAMELENGTH=`echo -n "$IMAGENAME" | wc -m`

if [ $NAMELENGTH = 0 ]; then
    docker container run -v $(pwd)/tmp:/home --rm -i --name pg-backup --network=host $(docker build --build-arg USER_ID=$(id -u) -f dev_tools/Dockerfile-backup -q .) /bin/sda-backup --action pg_bb-restore --name "$DBCOPY"
else
    docker container run -v $(pwd)/tmp:/home --rm -i --name pg-backup --network=host backup /bin/sda-backup --action pg_bb-restore --name "$DBCOPY"
fi

# Remove everything from the db
docker exec -i db /bin/sh -c "rm -r data/pgdata"

# Add the physical copy in the db container
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

# Find the user in tha db
USER=$(docker exec db psql -U postgres -d test -tA -c "select submission_user from local_ega.main where submission_file_path = 'test.c4gh';")

# Check if the user is the expected one
if [ "$USER" != "dummy" ]; then
    echo "Expected to get user 'dummy' but got '$USER'"
    exit 1
fi

# Remove the local folder
rm -r tmp
