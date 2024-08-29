#!/bin/bash

# Description:
#   Installs clusterctl to /usr/local/bin
#
# Usage:
#   $ install-clusterctl.sh

set -o errexit
set -o nounset
set -o pipefail

curl -L https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.8.1/clusterctl-linux-amd64 -o clusterctl
chmod +x ./clusterctl
sudo mv ./clusterctl /usr/local/bin

clusterctl version
