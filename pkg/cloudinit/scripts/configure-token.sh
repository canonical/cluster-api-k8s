#!/bin/bash -xe

## Assumptions:
## - k8s is installed and cluster is bootstrapped
## - /capi/etc/token contains the token CAPI providers can use to authenticate with k8sd

# TODO(neoaggelos): enable once API and CLI is implemented

# k8s x-capi set-auth-token "$(cat /capi/etc/token)"
