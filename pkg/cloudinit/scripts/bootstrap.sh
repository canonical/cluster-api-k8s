#!/bin/bash -xe

## Assumptions:
## - k8s is installed
## - /opt/capi/etc/microcluster-address contains the address to use for microcluster
## - /opt/capi/etc/config.yaml is a valid bootstrap configuration file

address="$(cat /opt/capi/etc/microcluster-address)"
config_file="/opt/capi/etc/config.yaml"

k8s bootstrap --address "${address}" --config "${config_file}"
