#!/bin/bash

# Description:
#   Installs clusterawsadm to /usr/local/bin
#
# Usage:
#   $ install-clusterawsadm.sh

set -o errexit
set -o nounset
set -o pipefail

curl -L https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases/download/v0.0.0/clusterawsadm-linux-amd64 -o clusterawsadm
chmod +x ./clusterawsadm
sudo mv ./clusterawsadm /usr/local/bin

clusterawsadm version
