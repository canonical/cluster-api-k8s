#!/bin/bash

# Description:
#   Installs aws-nuke to /usr/local/bin
#
# Usage:
#   $ install-aws-nuke.sh

set -o errexit
set -o nounset
set -o pipefail

curl -L https://github.com/rebuy-de/aws-nuke/releases/download/v2.25.0/aws-nuke-v2.25.0-linux-amd64.tar.gz -o aws-nuke-v2.25.0-linux-amd64.tar.gz
tar -xvf aws-nuke-v2.25.0-linux-amd64.tar.gz -C /tmp
rm aws-nuke-v2.25.0-linux-amd64.tar.gz
chmod +x /tmp/aws-nuke-v2.25.0-linux-amd64
mv /tmp/aws-nuke-v2.25.0-linux-amd64 /usr/local/bin/aws-nuke

aws-nuke version
