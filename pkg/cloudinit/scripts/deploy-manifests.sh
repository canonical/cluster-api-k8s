#!/bin/bash -xe

## Assumptions:
## - k8s is installed and bootstrapped.
## - /opt/capi/manifests/ is a directory with YAML manifests to deploy once on the cluster.

for file in $(find /opt/capi/manifests/ -name '*.yaml' | sort); do
  k8s kubectl apply -f "$file"
done
