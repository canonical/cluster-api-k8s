#!/bin/bash

# WARNING: DO NOT enable -x as it will expose sensitive information in the logs.
# Enable debugging selectively using set -x and set +x around specific code blocks.
set -euo pipefail

# This script is used to run e2e tests for the CK8s CAPI.
# It sets up an LXD container, installs the CK8s management cluster, and runs e2e tests.
# The goal is to test the CK8s provider with different infrastructure providers (e.g., AWS, Azure, GCP). Only AWS is supported for now.
# The script should be able to run on any Linux machine with LXD installed.

# USAGE
# ./hack/ci-e2e-tests.sh [INFRA_PROVIDER] [CK8S_PROVIDER_VERSION]
# INFRA_PROVIDER: Optional. The infrastructure provider to use. Default is "aws".
# CK8S_PROVIDER_VERSION: Optional. The CK8s provider version to use. Default is "v0.1.2".

readonly HACK_DIR="$(realpath $(dirname "${0}"))"
cd "$HACK_DIR"

readonly INFRA_PROVIDER=${2:-aws}
readonly CK8S_PROVIDER_VERSION=${3:-v0.1.2}

readonly LXD_CHANNEL="6.1/stable"
readonly LXC_IMAGE="ubuntu:22.04"
readonly K8S_PROFILE_URL="https://raw.githubusercontent.com/canonical/k8s-snap/main/tests/integration/lxd-profile.yaml"
readonly K8S_PROFILE_PATH="/tmp/k8s.profile"
readonly CONTAINER_NAME="k8s-test"

# Utility function for printing errors to stderr
function error_exit {
  printf "ERROR: %s\n" "$1" >&2
  return 1
}

function log_info {
  printf "INFO: %s\n" "$1"
}

# Check that all required environment variables are set
function check_required_env_vars {
  log_info "Checking required environment variables..."

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

function setup_firewall {
  log_info "Setting up firewall rules..."

  if sudo iptables -L DOCKER-USER; then
    sudo iptables -I DOCKER-USER -i lxdbr0 -j ACCEPT
    sudo iptables -I DOCKER-USER -o lxdbr0 -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
  fi
}

# Install LXD snap
function install_lxd {
  log_info "Installing LXD..."

  sudo snap install lxd --channel=$LXD_CHANNEL
  sudo lxd waitready
  sudo lxd init --auto
  sudo usermod --append --groups lxd "$USER"
}

# Create or ensure the k8s profile exists
function setup_lxd_profile {
  log_info "Setting up LXD profile..."

  lxc profile show k8s || lxc profile create k8s
  wget -q $K8S_PROFILE_URL -O $K8S_PROFILE_PATH
  cat $K8S_PROFILE_PATH | lxc profile edit k8s
  rm -f $K8S_PROFILE_PATH
}

# Setup and configure the container
function setup_container {
  log_info "Setting up LXD container..."

  lxc launch $LXC_IMAGE $CONTAINER_NAME -p default -p k8s

  # Wait for container to be ready to run commands
  until exec_in_container true; do
    sleep 1
  done

  exec_in_container "apt update && apt install -y snapd"
  exec_in_container "systemctl start snapd"
  exec_in_container "snap wait core seed.loaded"

  # Script is running from the hack directory, so push the entire directory to the container
  lxc file push -r .. $CONTAINER_NAME/root/ >/dev/null
}

function configure_container_env {
  log_info "Configuring container environment..."

  if [[ $INFRA_PROVIDER == "aws" ]]; then
    log_info "Configuring AWS credentials in container..."

    # Check for clusterawsadm binary
    exec_in_container "which clusterawsadm" || error_exit "clusterawsadm binary not found in container"

    lxc config set $CONTAINER_NAME environment.AWS_REGION "$AWS_REGION"
    lxc config set $CONTAINER_NAME environment.AWS_SECRET_ACCESS_KEY "$AWS_SECRET_ACCESS_KEY"
    lxc config set $CONTAINER_NAME environment.AWS_ACCESS_KEY_ID "$AWS_ACCESS_KEY_ID"

    if [[ -z $AWS_SESSION_TOKEN ]]; then
      log_info "AWS_SESSION_TOKEN not set. Skipping..."
    else
      lxc config set $CONTAINER_NAME environment.AWS_SESSION_TOKEN "$AWS_SESSION_TOKEN"
    fi

    # This command can fail if the stack already exists, so we ignore the error
    exec_in_container "clusterawsadm bootstrap iam create-cloudformation-stack" || true

    local aws_creds
    aws_creds=$(lxc exec "$CONTAINER_NAME" -- bash -c "clusterawsadm bootstrap credentials encode-as-profile")

    echo "::add-mask::$aws_creds" # Mask the credentials in the Github CI logs.
    lxc config set "$CONTAINER_NAME" environment.AWS_B64ENCODED_CREDENTIALS "$aws_creds"
  fi
}

# Main installation and configuration
function setup_management_cluster {
  log_info "Setting up management cluster..."
  exec_in_container "sudo snap install k8s --classic --edge"
  exec_in_container "sudo snap install go --classic"
  exec_in_container "mkdir -p /root/.kube"
  exec_in_container "sudo k8s bootstrap"
  exec_in_container "sudo k8s status --wait-ready"
  exec_in_container "sudo k8s config > /root/.kube/config"
}

function clone_repos {
  log_info "Cloning CK8s and CAPI repositories..."
  exec_in_container "git clone --depth 1 https://github.com/kubernetes-sigs/cluster-api-provider-aws /root/cluster-api-provider-aws"
  exec_in_container "git clone --depth 1 https://github.com/kubernetes-sigs/cluster-api /root/cluster-api"
}

# Transfer and execute scripts
function install_tools {
  log_info "Installing tools in container..."

  tools=(install-clusterctl.sh)
  packages=(make)
  snaps=(kubectl)

  if [[ $INFRA_PROVIDER == "aws" ]]; then
    tools+=(install-clusterctlawsadm.sh install-aws-nuke.sh)
  fi

  for script in "${tools[@]}"; do
    exec_in_container "chmod +x /root/cluster-api-k8s/hack/$script && /root/cluster-api-k8s/hack/$script"
  done

  for package in "${packages[@]}"; do
    exec_in_container "apt install -y $package"
  done

  for snap in "${snaps[@]}"; do
    exec_in_container "snap install $snap --classic"
  done
}

function init_clusterctl {
  log_info "Initializing clusterctl with $INFRA_PROVIDER infrastructure and CK8s $CK8S_PROVIDER_VERSION..."

  configure_container_env # Ensures that the right environment variables are set in the container

  exec_in_container "chmod +x /root/cluster-api-k8s/hack/write-provider-config.sh"
  exec_in_container "mkdir -p /root/.cluster-api"
  exec_in_container "/root/cluster-api-k8s/hack/write-provider-config.sh /root/.cluster-api/clusterctl.yaml $CK8S_PROVIDER_VERSION"

  exec_in_container "clusterctl init -i $INFRA_PROVIDER -b ck8s:$CK8S_PROVIDER_VERSION -c ck8s:$CK8S_PROVIDER_VERSION --config /root/.cluster-api/clusterctl.yaml"
}

function run_e2e_tests {
  log_info "Running e2e tests..."
  exec_in_container "cd /root/cluster-api-k8s && make USE_EXISTING_CLUSTER=true GINKGO_FOCUS=\"Workload cluster creation\" test-e2e"
}

function main {
  if [[ $INFRA_PROVIDER != "aws" ]]; then
    error_exit "Unsupported infrastructure provider: $INFRA_PROVIDER"
    exit 1
  fi

  check_required_env_vars
  install_lxd
  setup_lxd_profile
  setup_firewall
  setup_container
  setup_management_cluster
  clone_repos
  install_tools
  init_clusterctl
  run_e2e_tests

  log_info "E2E tests completed successfully."
}

main
