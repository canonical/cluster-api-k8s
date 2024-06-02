#!/bin/bash -xe

## Assumptions:
## - k8s is installed
## - /opt/capi/etc/microcluster-address contains the address to use for microcluster
## - /opt/capi/etc/join-token is a valid join token

address="$(cat /opt/capi/etc/microcluster-address)"
token="$(cat /opt/capi/etc/join-token)"

k8s join-cluster "${token}" --address "${address}"
