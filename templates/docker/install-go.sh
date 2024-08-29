#!/bin/bash -xe

# Author: Angelos Kolaitis <angelos.kolaitis@canonical.com>
#
# Usage:
#    $ install-go.sh 1.22.6
#
# Description: Download go version and install under /usr/local/go

VERSION="$1"

fname="go${VERSION}.linux-amd64.tar.gz"
wget "https://go.dev/dl/${fname}"
tar -C /usr/local -xvzf "${fname}"
rm "${fname}"
