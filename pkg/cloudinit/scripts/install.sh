#!/bin/bash -xe

## Assumptions:
## - /capi/etc/snap-track contains the snap track that matches the installed Kubernetes version, e.g. "v1.30.1" -> "1.30-classic/stable"
## - /capi/etc/snap-revision contains the snap revision to be installed, e.g. 123
## - /capi/etc/snap-local-path contains the path to the local snap file to be installed, e.g. /path/to/k8s.snap

if [ -f "/capi/etc/snap-track" ]; then
  snap_track="$(cat /capi/etc/snap-track)"
  snap install k8s --classic --channel "${snap_track}"
elif [ -f "/capi/etc/snap-revision" ]; then
  snap_revision="$(cat /capi/etc/snap-revision)"
  snap install k8s --classic --revision "${snap_revision}"
elif [ -f "/capi/etc/snap-local-path" ]; then
  snap_local_path="$(cat /capi/etc/snap-local-path)"
  snap install k8s --classic --dangerous "${snap_local_path}"
else
  echo "No snap installation option found"
  exit 1
fi
