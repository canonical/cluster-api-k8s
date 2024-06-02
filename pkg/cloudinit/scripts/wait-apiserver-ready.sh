#!/bin/bash -xe

## Assumptions:
## - k8s is installed and bootstrapped

while ! k8s kubectl get --raw /readyz; do
  echo "kube-apiserver not yet ready"
  sleep 1
done
