#!/bin/bash

docker exec db psql -U postgres -d test -c "INSERT INTO local_ega.main(submission_file_path,submission_user,submission_file_extension,status,encryption_method) VALUES('test.c4gh','dummy','c4gh','INIT','CRYPT4GH');"

CONFIGFILE="dev_tools/config_postgres.yaml" go run . --action pg_dump
# bail early since if this fails the rest will fail also
if [ $? != 0 ]; then
    exit 1
fi

docker exec db psql -U postgres -d postgres -c "DROP DATABASE test;"

docker exec db psql -U postgres -d postgres -c "CREATE DATABASE test;"

DUMPFILE=$(s3cmd -c dev_tools/s3conf ls s3://dumps/ | grep ".sqldump" | cut -d '/' -f4)
echo "restoring databse from file $DUMPFILE"
CONFIGFILE="dev_tools/config_postgres.yaml" go run . --action pg_restore --name "$DUMPFILE"

USER=$(docker exec db psql -U postgres -d test -tA -c "select elixir_id from local_ega.files where inbox_path = 'test.c4gh';")

if [ "$USER" != "dummy" ]; then
    echo "Expected to get user 'dummy' but got '$USER'"
    exit 1
fi
