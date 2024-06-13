#!/bin/bash

# Description:
#   Sync images from upstream repositories under ghcr.io/canonical.
#
# Usage:
#   $ USERNAME="$username" PASSWORD="$password" ./sync-images.sh

DIR="$(realpath "$(dirname "${0}")")"

"${DIR}/tools/regsync.sh" once -c "${DIR}/upstream-images.yaml"
