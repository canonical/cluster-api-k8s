#!/bin/bash -xe

# Author: Angelos Kolaitis <angelos.kolaitis@canonical.com>
#
# Usage:
#    $ install-go.sh 1.24.4-1
#
# Description: Download microsoft go version and install under /usr/local/go

# This version should come from https://github.com/microsoft/go/releases
VERSION="$1"

wget https://aka.ms/golang/release/latest/go$VERSION.linux-amd64.tar.gz
rm -rf /usr/local/go || true
tar -C /usr/local -xzvf go$VERSION.linux-amd64.tar.gz
rm go$VERSION.linux-amd64.tar.gz
