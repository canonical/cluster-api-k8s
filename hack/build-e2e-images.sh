#!/bin/bash

# Description:
#   Build k8s-snap docker images required for e2e tests.
#
# Usage:
#   ./build-e2e-images.sh
set -xe

DIR="$(realpath "$(dirname "${0}")")"

cd "${DIR}/../templates/docker"
sudo docker build . -t k8s-snap:dev-old --build-arg BRANCH=KU-4167/build-go-versions --build-arg KUBERNETES_VERSION=v1.33.4 --build-arg KUBERNETES_VERSION_UPGRADE_TO=v1.34.0
sudo docker build . -t k8s-snap:dev-new --build-arg BRANCH=KU-4167/build-go-versions --build-arg KUBERNETES_VERSION=v1.34.0
cd -
