#!/bin/bash -xe

## Assumptions:
## - k8s is installed and bootstrapped.
## - /capi/images/ is a directory with tar images that can be imported to containerd.
## - /capi/images/platform is an optional file with the platform name to specify when importing the image to containerd (e.g. "amd64")

platform="$(cat /capi/images/platform 2>/dev/null || true)"
for file in $(find /capi/images/ -name '*.tar' 2> /dev/null || true | sort); do
  /snap/k8s/current/bin/ctr --namespace k8s.io --address /var/snap/k8s/common/run/containerd.sock image import --platform "${platform}" "${file}"
done
