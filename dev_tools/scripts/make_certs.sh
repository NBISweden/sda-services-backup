#!/bin/sh

mkdir -p "$(dirname "$0")/../certs"

# create CA certificate
openssl req -config "$(dirname "$0")"/ssl.cnf -new -sha256 -nodes -extensions v3_ca -out $(dirname "$0")/../certs/ca.csr -keyout $(dirname "$0")/../certs/ca-key.pem
openssl req -config "$(dirname "$0")"/ssl.cnf -key $(dirname "$0")/../certs/ca-key.pem -x509 -new -days 7300 -sha256 -nodes -extensions v3_ca -out $(dirname "$0")/../certs/ca.pem

# Create certificate for servers
openssl req -config "$(dirname "$0")"/ssl.cnf -new -nodes -newkey rsa:4096 -keyout $(dirname "$0")/../certs/server.key -out $(dirname "$0")/../certs/server.csr -extensions server_cert
openssl x509 -req -in $(dirname "$0")/../certs/server.csr -days 1200 -CA $(dirname "$0")/../certs/ca.pem -CAkey $(dirname "$0")/../certs/ca-key.pem -set_serial 01 -out $(dirname "$0")/../certs/server.crt -extensions server_cert -extfile "$(dirname "$0")"/ssl.cnf

cat $(dirname "$0")/../certs/server.crt $(dirname "$0")/../certs/server.key > $(dirname "$0")/../certs/server.pem

chmod 644 $(dirname "$0")/../certs/*
