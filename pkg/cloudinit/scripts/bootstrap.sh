#!/bin/bash -xe

## Assumptions:
## - k8s is installed
## - /capi/etc/microcluster-address contains the address to use for microcluster
## - /capi/etc/config.yaml is a valid bootstrap configuration file

address="$(cat /capi/etc/microcluster-address)"
config_file="/capi/etc/config.yaml"

if [ ! -f /etc/kubernetes/pki/ca.crt ]; then
  k8s bootstrap --address "${address}" --file "${config_file}"
fi
