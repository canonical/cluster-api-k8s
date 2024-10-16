#!/bin/bash -xe

# Assumptions:
#   - runs before install k8s

#   - /capi/etc/http-proxy contains http proxy value
#   - /capi/etc/https-proxy contains https proxy value
#   - /capi/etc/no-proxy contains no proxy value


HTTP_PROXY=$(cat /capi/etc/http-proxy)
HTTPS_PROXY=$(cat /capi/etc/https-proxy)
NO_PROXY=$(cat /capi/etc/no-proxy)

ENVIRONMENT_FILE="/etc/environment"

if [[ "${HTTP_PROXY}" != "" ]]; then
  echo "http_proxy=${HTTP_PROXY}" >> "${ENVIRONMENT_FILE}"
  echo "HTTP_PROXY=${HTTP_PROXY}" >> "${ENVIRONMENT_FILE}"
fi

if [[ "${HTTPS_PROXY}" != "" ]]; then
  echo "https_proxy=${HTTPS_PROXY}" >> "${ENVIRONMENT_FILE}"
  echo "HTTPS_PROXY=${HTTPS_PROXY}" >> "${ENVIRONMENT_FILE}"
fi

if [[ "${NO_PROXY}" != "" ]]; then
  echo "no_proxy=${NO_PROXY}" >> "${ENVIRONMENT_FILE}"
  echo "NO_PROXY=${NO_PROXY}" >> "${ENVIRONMENT_FILE}"
fi
