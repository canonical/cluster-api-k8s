#!/bin/bash -xe

# Assumptions:
#   - snapd is installed
#   - /capi/etc/snapstore-proxy-scheme contains the snapstore scheme
#   - /capi/etc/snapstore-proxy-domain contains the snapstore domain
#   - /capi/etc/snapstore-proxy-id contains the snapstore id

if [ ! -s /capi/etc/snapstore-proxy-scheme ] || [ ! -s /capi/etc/snapstore-proxy-domain ] || [ ! -s /capi/etc/snapstore-proxy-id ]; then
  echo "Missing or empty snapstore proxy configuration files, exiting."
  exit 1
fi

SNAPSTORE_PROXY_SCHEME=$(cat /capi/etc/snapstore-proxy-scheme)
SNAPSTORE_PROXY_DOMAIN=$(cat /capi/etc/snapstore-proxy-domain)
SNAPSTORE_PROXY_ID=$(cat /capi/etc/snapstore-proxy-id)

if ! type -P curl; then
  count=0
  while ! snap install curl; do
    count=$((count + 1))
    if [ $count -gt 5 ]; then
      echo "Failed to install curl, exiting."
      exit 1
    fi
    echo "Failed to install curl, retrying ($count/5)"
    sleep 5
  done
fi

count=0
while ! curl -sL "${SNAPSTORE_PROXY_SCHEME}"://"${SNAPSTORE_PROXY_DOMAIN}"/v2/auth/store/assertions | snap ack /dev/stdin; do
  count=$((count + 1))
  if [ $count -ge 5 ]; then
    echo "Failed to ACK store assertions, exiting."
    exit 1
  fi

  echo "Failed to ACK store assertions, retrying ($count/5)"
  sleep 5
done

count=0
while ! snap set core proxy.store="${SNAPSTORE_PROXY_ID}"; do
  count=$((count + 1))
  if [ $count -ge 5 ]; then
    echo "Failed to configure snapd with store ID, exiting."
    exit 1
  fi

  echo "Failed to configure snapd with store ID, retrying ($count/5)"
  sleep 5
done

systemctl restart snapd
