#!/bin/bash
# Copyright 2025 Canonical Group Limited.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

# Retry function
# Usage: retry <max_attempts> <delay_seconds> <command...>
retry() {
  local max_attempts=$1
  local delay=$2
  shift 2
  local attempt=1
  
  while [ $attempt -le $max_attempts ]; do
    if "$@"; then
      return 0
    else
      if [ $attempt -lt $max_attempts ]; then
        echo "    Attempt $attempt failed. Retrying in ${delay}s..."
        sleep $delay
        attempt=$((attempt + 1))
      else
        echo "    All $max_attempts attempts failed."
        return 1
      fi
    fi
  done
}

# Description:
#   Setup the bootstrap cluster in a LXD container
# Usage:
#   ./hack/setup-management-cluster.sh <bootstrap-cluster-name> <k8s-version> [provider-images-path]
# Example:
#   ./hack/setup-management-cluster.sh bootstrap-cluster 1.32 provider-images.tar

if [ -z "${1:-}" ]; then
  echo "Error: bootstrap-cluster-name is required"
  echo "Usage: $0 <bootstrap-cluster-name> <k8s-version> [provider-images-path]"
  exit 1
fi

if [ -z "${2:-}" ]; then
  echo "Error: k8s-version is required"
  echo "Usage: $0 <bootstrap-cluster-name> <k8s-version> [provider-images-path]"
  exit 1
fi

bootstrap_cluster_name=${1}
bootstrap_cluster_version=${2}
provider_images_path=${3:-}

echo "==> Launching LXD container '$bootstrap_cluster_name' with Ubuntu 24.04..."
sudo lxc -p default -p k8s-integration launch ubuntu:24.04 $bootstrap_cluster_name

echo "==> Installing k8s snap (version $bootstrap_cluster_version)..."
retry 5 5 sudo lxc exec $bootstrap_cluster_name -- snap install k8s --classic --channel=$bootstrap_cluster_version-classic/stable

echo "==> Bootstrapping k8s cluster..."
retry 5 5 sudo lxc exec $bootstrap_cluster_name -- k8s bootstrap

if [ -n "$provider_images_path" ]; then
  echo "==> Pushing provider images to container..."
  sudo lxc file push $provider_images_path $bootstrap_cluster_name/root/provider-images.tar

  echo "==> Loading provider images into containerd..."
  sudo lxc exec $bootstrap_cluster_name -- /snap/k8s/current/bin/ctr -n k8s.io images import /root/provider-images.tar
fi

echo "==> Getting bootstrap cluster IP address..."
bootstrap_cluster_ip=$(sudo lxc exec $bootstrap_cluster_name -- bash -c "ip -4 addr show eth0 | grep -oP '(?<=inet\s)\d+(\.\d+){3}'")
echo "    Bootstrap cluster IP: $bootstrap_cluster_ip"

echo "==> Creating cluster-info configmap..."
kubeconfig="apiVersion: v1
clusters:
- cluster:
    server: https://${bootstrap_cluster_ip}:6443
  name: ""
contexts: null
current-context: ""
kind: Config
users: null"

echo "    Creating temporary kubeconfig file..."
temp_kubeconfig=$(mktemp)
echo "$kubeconfig" > "$temp_kubeconfig"

echo "    Pushing kubeconfig $temp_kubeconfig to container at /tmp/$bootstrap_cluster_name-cluster-info.yaml..."
sudo lxc file push "$temp_kubeconfig" "$bootstrap_cluster_name/tmp/$bootstrap_cluster_name-cluster-info.yaml"

echo "    Creating cluster-info configmap in kube-public namespace..."
sudo lxc exec $bootstrap_cluster_name -- k8s kubectl create configmap cluster-info -n kube-public --from-file=kubeconfig=/tmp/$bootstrap_cluster_name-cluster-info.yaml

echo "    Cleaning up temporary files..."
rm "$temp_kubeconfig"
sudo lxc exec $bootstrap_cluster_name -- rm /tmp/$bootstrap_cluster_name-cluster-info.yaml

echo "==> Setting up kubeconfig..."
sudo lxc exec $bootstrap_cluster_name -- mkdir -p /root/.kube
sudo lxc exec $bootstrap_cluster_name -- bash -c "k8s config > /root/.kube/config"

echo "==> Pulling kubeconfig from $bootstrap_cluster_name to ~/.kube/config..."
mkdir -p ~/.kube
sudo lxc file pull $bootstrap_cluster_name/root/.kube/config ~/.kube/config

echo "==> Setup complete! Bootstrap cluster '$bootstrap_cluster_name' is ready."




