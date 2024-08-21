#!/bin/bash -xe

# Author: Angelos Kolaitis <angelos.kolaitis@canonical.com>
#
# Usage:
#    $ install-go.sh $version
#
# Description: Download go version and install under /usr/local/go

VERSION="$1"
wget "https://go.dev/dl/go1.22.6.linux-amd64.tar.gz"
tar -C /usr/local -xvzf "go1.22.6.linux-amd64.tar.gz"
rm "go${VERSION}.linux-amd64.tar.gz"
