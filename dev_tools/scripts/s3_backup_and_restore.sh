#!/bin/bash

for n in "destination" "source"; do
    s3cmd -c dev_tools/s3conf mb "s3://$n"
done

set -e

for i in {0..9}; do
    dd if=/dev/urandom bs=1M count=16 of=/tmp/mock-data"$i" status=none
    s3cmd -q -c dev_tools/s3conf put "/tmp/mock-data$i" "s3://source/wals/0000000900000001/00000009000000010000005$i"
done

## bakup and encrypt data to the configured bucket
CONFIGFILE="dev_tools/config_s3_backup.yaml" go run . --action backup_bucket

## restore data to a new bucket
DESTINATION_BUCKET="restored" SOURCE_BUCKET="encrypted" CONFIGFILE="dev_tools/config_s3_backup.yaml" go run . --action restore_bucket
s3cmd -q -c dev_tools/s3conf get s3://restored/wals/0000000900000001/000000090000000100000059 /tmp/restored --force
if ! cmp --silent /tmp/mock-data9 /tmp/restored; then
    echo "restored file differs from original"
    exit 1
fi

## sync objects between buckets
DESTINATION_BUCKET="sync" CONFIGFILE="dev_tools/config_s3_backup.yaml" go run . --action sync_buckets

synced_sha=$(s3cmd -c dev_tools/s3conf --no-progress get s3://sync/wals/0000000900000001/000000090000000100000055 - | sha256sum | cut -d ' ' -f1)
org_sha=$(sha256sum /tmp/mock-data5 | cut -d ' ' -f1)
if ! [ "$org_sha" == "$synced_sha" ]; then
    echo "synced files not identical, $org_sha != $synced_sha"
    exit 1
fi
