#!/bin/bash -xe

## Assumptions:
## - /capi/etc/snap-track contains the snap track that matches the installed Kubernetes version, e.g. "1.30.1" -> "1.30-classic/stable"

snap install k8s --classic --channel "${1}"
