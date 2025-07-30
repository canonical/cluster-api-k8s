#!/usr/bin/bash

# https://capn.linuxcontainers.org/tutorial/quick-start.html
ip_address="$(ip -o route get to 1.1.1.1 | sed -n 's/.*src \([0-9.]\+\).*/\1/p')"
sudo lxc config set core.https_address "$ip_address:8443"

token="$(sudo lxc config trust add --name client | tail -1)"
sudo lxc remote add local-https --token "$token" "https://$(sudo lxc config get core.https_address)"
