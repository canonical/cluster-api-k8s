#!/bin/bash -xe

## Assumptions:
## - /capi/etc/snap-channel contains the snap channel to be installed that matches the desired Kubernetes version, e.g. "v1.30.1" -> "1.30-classic/stable"
## - /capi/etc/snap-revision contains the snap revision to be installed, e.g. 123
## - /capi/etc/snap-local-path contains the path to the local snap file to be installed (e.g. /path/to/k8s.snap),
##   or the path to a folder containing the local snap files to be installed (e.g. /path/to)

# Function to retry snap installation with a maximum number of attempts
# and a delay between attempts. This is useful in case of transient errors
retry_snap_install() {
  local max_attempts=5
  local delay=3
  local attempt=1
  
  while [ $attempt -le $max_attempts ]; do
    echo "Attempt $attempt of $max_attempts to install snap..."
    if "$@"; then
      echo "Snap installation succeeded"
      return 0
    else
      echo "Snap installation failed"
      if [ $attempt -lt $max_attempts ]; then
        echo "Retrying in $delay seconds..."
        sleep $delay
      fi
    fi
    attempt=$((attempt + 1))
  done
  
  echo "Failed to install snap after $max_attempts attempts"
  return 1
}

if [ -f "/capi/etc/snap-channel" ]; then
  snap_channel="$(cat /capi/etc/snap-channel)"
  retry_snap_install snap install k8s --classic --channel "${snap_channel}"
elif [ -f "/capi/etc/snap-revision" ]; then
  snap_revision="$(cat /capi/etc/snap-revision)"
  retry_snap_install snap install k8s --classic --revision "${snap_revision}"
elif [ -f "/capi/etc/snap-local-path" ]; then
  snap_local_path="$(cat /capi/etc/snap-local-path)"
  snap_local_paths=( "${snap_local_path}" )

  # If $snap_local_path is a folder, install all the snaps from that folder.
  if [[ -d "${snap_local_path}" ]]; then
    snap_local_paths=($(ls ${snap_local_path}/*.snap))
  fi
  ls ${snap_local_path}/*.assert | xargs -L 1 snap ack
  retry_snap_install snap install --classic "${snap_local_paths[@]}"
else
  echo "No snap installation option found"
  exit 1
fi
