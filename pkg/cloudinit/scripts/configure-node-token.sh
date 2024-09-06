#!/bin/bash -xe

## Assumptions:
## - k8s is installed and cluster is bootstrapped
## - /capi/etc/node-token contains the token CAPI providers can use to authenticate with k8sd for per-node operations

k8s x-capi set-node-token "$(cat /capi/etc/node-token)"
