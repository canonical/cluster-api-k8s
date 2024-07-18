#!/bin/bash -xe

# Author: Angelos Kolaitis <angelos.kolaitis@canonical.com>
#
# Usage:
#    $ install-go.sh 1.22
#
# Description: Download latest go version and install under /usr/local/go

VERSION="$1"

fname="$(curl -s https://go.dev/dl/ | grep -o "go$VERSION.*.linux-amd64.tar.gz" | head -1)"
wget "https://go.dev/dl/$fname"
tar -C /usr/local -xvzf "$fname"
rm "$fname"
