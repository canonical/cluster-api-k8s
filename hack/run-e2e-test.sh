#!/bin/bash

snap install go --classic --channel 1.22/stable
snap install kubectl --classic --channel 1.31/stable

apt update
apt install -y docker.io docker-buildx make
systemctl enable --now docker

curl -L https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases/download/v2.6.1/clusterawsadm-linux-amd64 -o clusterawsadm
chmod +x ./clusterawsadm
mv ./clusterawsadm /usr/local/bin
clusterawsadm version

wget https://github.com/kubernetes-sigs/kind/releases/download/v0.24.0/kind-linux-amd64 -O /usr/local/bin/kind

export KIND_EXPERIMENTAL_DOCKER_NETWORK=bridge
kind version

docker pull ghcr.io/canonical/cluster-api-k8s/bootstrap-controller:ci-test
docker tag ghcr.io/canonical/cluster-api-k8s/bootstrap-controller:ci-test ghcr.io/canonical/cluster-api-k8s/bootstrap-controller:dev
docker pull ghcr.io/canonical/cluster-api-k8s/controlplane-controller:ci-test
docker tag ghcr.io/canonical/cluster-api-k8s/controlplane-controller:ci-test ghcr.io/canonical/cluster-api-k8s/controlplane-controller:dev

git clone git@github.com:canonical/cluster-api-k8s.git /home/ubuntu/cluster-api-k8s && (cd /home/ubuntu/cluster-api-k8s || exit 1)

sudo -E E2E_INFRA=aws GINKGO_FOCUS="Workload cluster creation" SKIP_RESOURCE_CLEANUP=true make test-e2e
