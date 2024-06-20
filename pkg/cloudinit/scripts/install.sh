#!/bin/bash -xe

## Assumptions:
## - /capi/etc/snap-track contains the snap track that matches the installed Kubernetes version, e.g. "v1.30.1" -> "1.30-classic/stable"

snap_track="$(cat /capi/etc/snap-track)"

snap install k8s --classic --channel "${snap_track}"
