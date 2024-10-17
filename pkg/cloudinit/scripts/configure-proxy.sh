#!/bin/bash -e

# Assumptions:
#   - runs before install k8s

HTTP_PROXY_FILE="/capi/etc/http-proxy"
HTTPS_PROXY_FILE="/capi/etc/https-proxy"
NO_PROXY_FILE="/capi/etc/no-proxy"
ENVIRONMENT_FILE="/etc/environment"



if [ -f ${HTTP_PROXY_FILE} ]; then
  HTTP_PROXY=$(cat ${HTTP_PROXY_FILE})
  echo "http_proxy=${HTTP_PROXY}" >> "${ENVIRONMENT_FILE}"
  echo "HTTP_PROXY=${HTTP_PROXY}" >> "${ENVIRONMENT_FILE}"
fi


if [ -f ${HTTPS_PROXY_FILE} ]; then
  HTTPS_PROXY=$(cat ${HTTPS_PROXY_FILE})
  echo "https_proxy=${HTTPS_PROXY}" >> "${ENVIRONMENT_FILE}"
  echo "HTTPS_PROXY=${HTTPS_PROXY}" >> "${ENVIRONMENT_FILE}"
fi


if [ -f ${NO_PROXY_FILE} ]; then
  NO_PROXY=$(cat ${NO_PROXY_FILE})
  echo "no_proxy=${NO_PROXY}" >> "${ENVIRONMENT_FILE}"
  echo "NO_PROXY=${NO_PROXY}" >> "${ENVIRONMENT_FILE}"
fi
