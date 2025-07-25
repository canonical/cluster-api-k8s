#!/bin/bash -xe

## Assumptions:
## - k8s is installed
## - /capi/etc/microcluster-address contains the address to use for microcluster
## - /capi/etc/config.yaml is a valid bootstrap configuration file

address="$(cat /capi/etc/microcluster-address)"
name="$(cat /capi/etc/node-name)"
config_file="/capi/etc/config.yaml"

if [ ! -f /etc/kubernetes/pki/ca.crt ]; then
  /snap/bin/k8s bootstrap --name "${name}" --address "${address}" --file "${config_file}"
fi
