#!/usr/bin/bash
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

# https://capn.linuxcontainers.org/tutorial/quick-start.html
ip_address="$(ip -o route get to 1.1.1.1 | sed -n 's/.*src \([0-9.]\+\).*/\1/p')"

sudo lxd init --auto --network-address "$ip_address"
sudo lxc network set lxdbr0 ipv6.address=none
sudo lxc cluster enable "$ip_address"

token="$(sudo lxc config trust add --name client | tail -1)"
sudo lxc remote add local-https --token "$token" "https://$(sudo lxc config get core.https_address)"
sudo lxc remote set-default local-https

wget https://raw.githubusercontent.com/canonical/k8s-snap/refs/heads/main/tests/integration/lxd-profile.yaml

sudo lxc profile create k8s-integration < lxd-profile.yaml
