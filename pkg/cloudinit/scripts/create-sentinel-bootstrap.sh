#!/bin/bash -xe

## Assumptions:
## - k8s is installed and cluster is bootstrapped

mkdir -p /run/cluster-api
touch /run/cluster-api/bootstrap-success.complete
