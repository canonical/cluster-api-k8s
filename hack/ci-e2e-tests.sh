#!/bin/bash

set -xe

readonly HACK_DIR="$(realpath $(dirname "${0}"))"

cd "$HACK_DIR"

readonly SKIP_CLEANUP=${1:-true}
readonly
readonly LXD_CHANNEL="5.21/stable"
readonly LXC_IMAGE="ubuntu:20.04"
readonly K8S_PROFILE_URL="https://raw.githubusercontent.com/canonical/k8s-snap/main/tests/integration/lxd-profile.yaml"
readonly K8S_PROFILE_PATH="/tmp/k8s.profile"
readonly CONTAINER_NAME="k8s-test"

# User specific paths
readonly USER_CREDENTIALS_PATH="/home/user/.creds"

# Install LXD snap
function install_lxd {
  sudo snap install lxd --channel=$LXD_CHANNEL
  sudo lxd init --auto
  sudo usermod --append --groups lxd "$USER"
}

# Create or ensure the k8s profile exists
function setup_lxd_profile {
  lxc profile create k8s || true
  wget -q $K8S_PROFILE_URL -O $K8S_PROFILE_PATH
  cat $K8S_PROFILE_PATH | lxc profile edit k8s
  rm -f $K8S_PROFILE_PATH
}

# Setup and configure the container
function setup_container {
  lxc launch $LXC_IMAGE $CONTAINER_NAME -p default -p k8s
  sleep 3 # Wait for the container to be up and running
  lxc exec $CONTAINER_NAME -- bash -c "apt update && apt install -y snapd"
  sleep 1
  lxc exec $CONTAINER_NAME -- bash -c "systemctl start snapd"
}

# Main installation and configuration
function setup_management_cluster {
  lxc exec $CONTAINER_NAME -- bash -c "snap install k8s --classic --edge"
  sleep 1
  lxc exec $CONTAINER_NAME -- bash -c "snap install go --classic"
  lxc exec $CONTAINER_NAME -- bash -c "mkdir -p /root/.kube"
  lxc exec $CONTAINER_NAME -- bash -c "sudo k8s bootstrap"
  lxc exec $CONTAINER_NAME -- bash -c "sudo k8s status --wait-ready"
  lxc exec $CONTAINER_NAME -- bash -c "sudo k8s config > /root/.kube/config"
}

# Transfer and execute scripts
function install_tools {
  for script in install-clusterctl.sh install-clusterctlawsadm.sh install-aws-nuke.sh write-provider-config.sh; do
    lxc file push ./$script $CONTAINER_NAME/root/$script
    if [[ ! $script == "write-provider-config.sh" ]]; then
      lxc exec $CONTAINER_NAME -- bash -c "chmod +x /root/$script && /root/$script"
    else
      lxc exec $CONTAINER_NAME -- bash -c "mkdir -p /root/.cluster-api"
      lxc exec $CONTAINER_NAME -- bash -c "chmod +x /root/$script && /root/$script /root/.cluster-api/clusterctl.yaml v0.1.2"
    fi
  done

  lxc file push $USER_CREDENTIALS_PATH $CONTAINER_NAME/root/.creds
  lxc exec $CONTAINER_NAME -- bash -c 'echo "source /root/.creds" >/root/.bashrc'
}

function init_clusterctl {
  lxc exec $CONTAINER_NAME -- bash -c "source /root/.creds && clusterctl init -i aws -b ck8s:v0.1.2 -c ck8s:v0.1.2 --config /root/.cluster-api/clusterctl.yaml"
}

function run_e2e_tests {
  make GINKGO.FOCUS="Workload cluster creation" test-e2e
}

function write_aws_nuke_config {
  lxc exec $CONTAINER_NAME -- bash -c "mkdir -p /root/.aws-nuke"
  lxc exec $CONTAINER_NAME -- bash -c "echo ""$AWS_NUKE_CONFIG"" > /root/.aws-nuke/config.yaml"
}

function cleanup {
  write_aws_nuke_config
  lxc exec $CONTAINER_NAME -- bash -c "aws-nuke --config /root/.aws-nuke/config.yaml --force"
  lxc delete $CONTAINER_NAME --force
}

function main {
  install_lxd
  setup_lxd_profile
  setup_container
  setup_management_cluster
  install_tools
  init_clusterctl
  run_e2e_tests

  if [[ $SKIP_CLEANUP == "false" ]]; then
    write_aws_nuke_config
    cleanup
  fi
}

main
