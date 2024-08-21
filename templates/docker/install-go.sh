#!/bin/bash -xe

# Author: Angelos Kolaitis <angelos.kolaitis@canonical.com>
#
# Usage:
#    $ install-go.sh
#
# Description: Download go 1.22.6 and install under /usr/local/go

wget "https://go.dev/dl/go1.22.6.linux-amd64.tar.gz"
tar -C /usr/local -xvzf "go1.22.6.linux-amd64.tar.gz"
rm "go1.22.6.linux-amd64.tar.gz"
