#!/bin/bash

# Description:
#   Build k8s-snap docker images required for e2e tests.
#
# Usage:
#   ./build-e2e-images.sh
set -xe

DIR="$(realpath "$(dirname "${0}")")"

cd "${DIR}/../templates/docker"
sudo docker build . -t k8s-snap:dev-old --build-arg REPO=https://github.com/claudiubelu/k8s-snap --build-arg BRANCH=stability-test --build-arg KUBERNETES_VERSION=v1.29.6 --build-arg KUBERNETES_VERSION_UPGRADE_TO=v1.30.4
sudo docker build . -t k8s-snap:dev-new --build-arg REPO=https://github.com/claudiubelu/k8s-snap --build-arg BRANCH=stability-test --build-arg KUBERNETES_VERSION=v1.30.4
cd -
