#!/bin/bash -xe

## Assumptions:
## - k8s is installed
## - /capi/etc/microcluster-address contains the address to use for microcluster
## - /capi/etc/join-token is a valid join token

address="$(cat /capi/etc/microcluster-address)"
name="$(cat /capi/etc/node-name)"
config_file="/capi/etc/config.yaml"
token="$(cat /capi/etc/join-token)"

/snap/bin/k8s join-cluster "${token}" --name "${name}" --address "${address}" --file "${config_file}"
