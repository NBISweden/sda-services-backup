#!/bin/bash

LOCATION=$(dirname "$0")/..

mkdir -p "$LOCATION/keys"

# Check if curl or wget is installed
get=""
if [ "$(command -v curl)" ]; then
    get="curl"
elif [ "$(command -v wget)" ]; then
    get="wget"
else
    echo "Neither curl or wget found, exiting"
    exit 1
fi

# Check if crypt4gh is installed and if it is the golang version
C4GH=$(command -v crypt4gh)
C4GHGEN=$(crypt4gh generate) >/dev/null 2>&1

# Check the system information
ARCH=$(uname -sm | sed 's/ /_/' | tr '[:upper:]' '[:lower:]')

# If crypt4gh is not installed or if it is not the golang version, download the golang version
if [ -z "$C4GH" ] | [[ $C4GHGEN != *"the required flag"* ]]; then
    echo "crypt4gh golang version not found, downloading v1.8.2"
    if [ $get == "curl" ]; then
        curl -sL "https://github.com/neicnordic/crypt4gh/releases/download/v1.8.2/crypt4gh_$ARCH.tar.gz" | tar zxf - -C "$LOCATION"/keys
    else
        wget -qO- "https://github.com/neicnordic/crypt4gh/releases/download/v1.8.2/crypt4gh_$ARCH.tar.gz" | tar zxf - -C "$LOCATION"/keys
    fi

    # Generate the keys and move them to the keys folder
    ./"$LOCATION"/keys/crypt4gh generate -n "$LOCATION/keys/backup" -p "randomPass"
else
    crypt4gh generate -n "$LOCATION/keys/backup" -p "randomPass"
fi
