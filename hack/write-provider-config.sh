#!/bin/bash

# Description:
#   Creates a clusterctl configuration file
#
# Usage:
#   $ write-clusterctl-config.sh $output-file $version

set -o errexit
set -o nounset
set -o pipefail

$OUTPUT_FILE=$1
$VERSION=$2

cat << EOF > "$OUTPUT_FILE"
providers:
  - name: "ck8s"
    url: "https://github.com/canonical/cluster-api-k8s/releases/download/${VERSION}/bootstrap-components.yaml"
    type: "BootstrapProvider"
  - name: "ck8s"
    url: "https://github.com/canonical/cluster-api-k8s/releases/download/${VERSION}/control-plane-components.yaml"
    type: "ControlPlaneProvider"
EOF
