#!/bin/bash

# Description:
#   Installs aws-nuke to /usr/local/bin
#
# Usage:
#   $ install-aws-nuke.sh

set -o errexit
set -o nounset
set -o pipefail

curl -L https://github.com/ekristen/aws-nuke/releases/download/v3.28.0/aws-nuke-v3.28.0-linux-amd64.tar.gz -o aws-nuke-v3.28.0-linux-amd64.tar.gz
tar -xvf aws-nuke-v3.28.0-linux-amd64.tar.gz -C /tmp
rm aws-nuke-v3.28.0-linux-amd64.tar.gz
chmod +x /tmp/aws-nuke
mv /tmp/aws-nuke /usr/local/bin/aws-nuke

aws-nuke version
