#!/bin/bash -xe

## Assumptions:
## - k8s is installed and bootstrapped.
## - /capi/manifests/ is a directory with YAML manifests to deploy once on the cluster.

for file in $(find /capi/manifests/ -name '*.yaml' || true | sort); do
  /snap/bin/k8s kubectl apply -f "$file"
done
