#!/bin/bash -xe

# Assumptions:
#   - snapd is installed
#   - /capi/etc/snapstore-proxy-scheme contains the snapstore scheme
#   - /capi/etc/snapstore-proxy-domain contains the snapstore domain
#   - /capi/etc/snapstore-proxy-id contains the snapstore id

SNAPSTORE_PROXY_SCHEME=$(cat /capi/etc/snapstore-proxy-scheme)
SNAPSTORE_PROXY_DOMAIN=$(cat /capi/etc/snapstore-proxy-domain)
SNAPSTORE_PROXY_ID=$(cat /capi/etc/snapstore-proxy-id)

if [ -z "${SNAPSTORE_PROXY_SCHEME}" ] || [ -z "${SNAPSTORE_PROXY_DOMAIN}" ] || [ -z "${SNAPSTORE_PROXY_ID}" ]; then
  echo "Missing snapstore proxy configuration"
  exit 1
fi

if ! type -P curl; then
  while ! snap install curl; do
    echo "Failed to install curl, will retry"
    sleep 5
  done
fi

while ! curl -sL "${SNAPSTORE_PROXY_SCHEME}"://"${SNAPSTORE_PROXY_DOMAIN}"/v2/auth/store/assertions | snap ack /dev/stdin; do
  echo "Failed to ACK store assertions, will retry"
  sleep 5
done

while ! snap set core proxy.store="${SNAPSTORE_PROXY_ID}"; do
  echo "Failed to configure snapd with store ID, will retry"
  sleep 5
done

systemctl restart snapd
