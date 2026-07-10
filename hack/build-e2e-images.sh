#!/bin/bash

# Description:
#   Build k8s-snap docker images required for e2e tests.
#
# Usage:
#   ./build-e2e-images.sh
#
# Environment:
#   DOCKER    docker command to use. Defaults to the local "docker" (rootless /
#             Colima friendly). For a rootful daemon, run: DOCKER="sudo docker" ./build-e2e-images.sh
#   PLATFORM  target platform for the build. Defaults to linux/amd64.
#             The k8s-snap / dqlite C build does not compile natively on arm64
#             (e.g. Apple Silicon), so we build amd64 by default. On an arm64 host
#             this builds under emulation (slow); see docs/e2e-node-images.md.
set -xe

DIR="$(realpath "$(dirname "${0}")")"
DOCKER="${DOCKER:-docker}"
PLATFORM="${PLATFORM:-linux/amd64}"

cd "${DIR}/../templates/docker"
# Build from the k8s-snap release branch matching each Kubernetes minor version
# (the moving "main" branch drifts from the Dockerfile's expected source layout).
${DOCKER} build . --platform "${PLATFORM}" -t k8s-snap:dev-old --build-arg BRANCH=release-1.32 --build-arg KUBERNETES_VERSION=v1.32.1 --build-arg KUBERNETES_VERSION_UPGRADE_TO=v1.33.0
${DOCKER} build . --platform "${PLATFORM}" -t k8s-snap:dev-new --build-arg BRANCH=release-1.33 --build-arg KUBERNETES_VERSION=v1.33.0
cd -
