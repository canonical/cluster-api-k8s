#!/bin/bash

set -xe

# This script is used to run e2e tests for the CK8s CAPI.
# It sets up an LXD container, installs the CK8s management cluster, and runs e2e tests.
# The goal is to test the CK8s provider with different infrastructure providers (e.g., AWS, Azure, GCP). Only AWS is supported for now.
# The script should be able to run on any Linux machine with LXD installed.

# USAGE
# ./hack/ci-e2e-tests.sh [SKIP_CLEANUP] [INFRA_PROVIDER] [CK8S_PROVIDER_VERSION]
# SKIP_CLEANUP: Optional. If set to "true", the LXD container and cloud provider resources will not be deleted after the tests are run. Default is "true".
# INFRA_PROVIDER: Optional. The infrastructure provider to use. Default is "aws".
# CK8S_PROVIDER_VERSION: Optional. The CK8s provider version to use. Default is "v0.1.2".

readonly HACK_DIR="$(realpath $(dirname "${0}"))"
cd "$HACK_DIR"

readonly SKIP_CLEANUP=${1:-true}
readonly INFRA_PROVIDER=${2:-aws}
readonly CK8S_PROVIDER_VERSION=${3:-v0.1.2}

readonly LXD_CHANNEL="5.21/stable"
readonly LXC_IMAGE="ubuntu:20.04"
readonly K8S_PROFILE_URL="https://raw.githubusercontent.com/canonical/k8s-snap/main/tests/integration/lxd-profile.yaml"
readonly K8S_PROFILE_PATH="/tmp/k8s.profile"
readonly CONTAINER_NAME="k8s-test"

# Utility function for printing errors to stderr
function error_exit {
  printf "ERROR: %s\n" "$1" >&2
  return 1
}

# Check that all required environment variables are set
function check_required_env_vars {
  local required_env_vars=()

  if [[ $INFRA_PROVIDER == "aws" ]]; then
    required_env_vars+=("AWS_REGION" "AWS_ACCESS_KEY_ID" "AWS_SECRET_ACCESS_KEY")
  fi

  for var in "${required_env_vars[@]}"; do
    if [ -z "${!var}" ]; then
      error_exit "Missing required environment variable: $var"
    fi
  done
}

function exec_in_container {
  lxc exec $CONTAINER_NAME -- bash -c "$1"
}

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
  until exec_in_container true; do
    sleep 1
  done

  exec_in_container "apt update && apt install -y snapd"
  exec_in_container "systemctl start snapd"
}

function configure_container_env {
  if [[ $INFRA_PROVIDER == "aws" ]]; then
    # Check for clusterawsadm binary
    exec_in_container "which clusterawsadm" || error_exit "clusterawsadm binary not found in container"

    set +x
    lxc config set $CONTAINER_NAME environment.AWS_REGION "$AWS_REGION"
    lxc config set $CONTAINER_NAME environment.AWS_SECRET_ACCESS_KEY "$AWS_SECRET_ACCESS_KEY"
    lxc config set $CONTAINER_NAME environment.AWS_ACCESS_KEY_ID "$AWS_ACCESS_KEY_ID"

    local aws_creds
    aws_creds=$(lxc exec "$CONTAINER_NAME" -- bash -c "clusterawsadm bootstrap credentials encode-as-profile")

    lxc config set "$CONTAINER_NAME" environment.AWS_B64ENCODED_CREDENTIALS "$aws_creds"
    set -x
  fi
}

# Main installation and configuration
function setup_management_cluster {
  sleep 5
  exec_in_container "snap install k8s --classic --edge"
  sleep 1
  exec_in_container "snap install go --classic"
  exec_in_container "mkdir -p /root/.kube"
  exec_in_container "sudo k8s bootstrap"
  exec_in_container "sudo k8s status --wait-ready"
  exec_in_container "sudo k8s config > /root/.kube/config"
}

# Transfer and execute scripts
function install_tools {
  tools=(install-clusterctl.sh)

  if [[ $INFRA_PROVIDER == "aws" ]]; then
    tools+=(install-clusterctlawsadm.sh install-aws-nuke.sh)
  fi

  for script in "${tools[@]}"; do
    lxc file push ./"$script" $CONTAINER_NAME/root/"$script"
    exec_in_container "chmod +x /root/$script && /root/$script"
  done
}

function init_clusterctl {
  configure_container_env # Ensures that the right environment variables are set in the container

  lxc file push ./write-provider-config.sh $CONTAINER_NAME/root/write-provider-config.sh
  exec_in_container "chmod +x /root/write-provider-config.sh"
  exec_in_container "mkdir -p /root/.cluster-api"
  exec_in_container "/root/write-provider-config.sh /root/.cluster-api/clusterctl.yaml $CK8S_PROVIDER_VERSION"

  exec_in_container "clusterctl init -i $INFRA_PROVIDER -b ck8s:$CK8S_PROVIDER_VERSION -c ck8s:$CK8S_PROVIDER_VERSION --config /root/.cluster-api/clusterctl.yaml"
}

function run_e2e_tests {
  make USE_EXISTING_CLUSTER=true GINKGO_FOCUS="Workload cluster creation" test-e2e
}

function cleanup {
  if [[ $SKIP_CLEANUP == "true" ]]; then
    return
  fi

  # Infra-specific cleanup
  if [[ $INFRA_PROVIDER == "aws" ]]; then
    exec_in_container "mkdir -p /root/.aws-nuke"
    exec_in_container "echo ""$AWS_NUKE_CONFIG"" > /root/.aws-nuke/config.yaml"
    exec_in_container "aws-nuke --config /root/.aws-nuke/config.yaml --force"
  fi

  lxc delete $CONTAINER_NAME --force
}

function main {
  if [[ $INFRA_PROVIDER != "aws" ]]; then
    error_exit "Unsupported infrastructure provider: $INFRA_PROVIDER"
    exit 1
  fi

  check_required_env_vars
  install_lxd
  setup_lxd_profile
  setup_container
  setup_management_cluster
  install_tools
  init_clusterctl
  #run_e2e_tests
  cleanup
}

main
