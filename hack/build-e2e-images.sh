#!/bin/bash

# Description:
#   Build k8s-snap docker images required for e2e tests.
#
# Usage:
#   ./build-e2e-images.sh

DIR="$(realpath "$(dirname "${0}")")"

cd "${DIR}/../templates/docker"
sudo docker build . -t k8s-snap:dev-old --build-arg BRANCH=main --build-arg KUBERNETES_VERSION=v1.29.6
sudo docker build . -t k8s-snap:dev-new --build-arg BRANCH=main --build-arg KUBERNETES_VERSION=v1.30.4
cd -
