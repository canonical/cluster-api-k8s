#!/bin/bash -xe

# Assumptions:
#   - k8s is installed

#   - /capi/etc/containerd-http-proxy contains containerd http proxy value
#   - /capi/etc/containerd-https-proxy contains containerd https proxy value
#   - /capi/etc/containerd-no-proxy contains containerd no proxy value


HTTP_PROXY=$(cat /capi/etc/containerd-http-proxy)
HTTPS_PROXY=$(cat /capi/etc/containerd-https-proxy)
NO_PROXY=$(cat /capi/etc/containerd-no-proxy)

mkdir -p /etc/systemd/system/snap.k8s.containerd.service.d
CONTAINERD_HTTP_PROXY="/etc/systemd/system/snap.k8s.containerd.service.d/http-proxy.conf"

echo "[Service]" >> "${CONTAINERD_HTTP_PROXY}"
need_restart=false



if [[ "${HTTP_PROXY}" != "" ]]; then
  echo "Environment=\"http_proxy=${HTTP_PROXY}\"" >> "${CONTAINERD_HTTP_PROXY}"
  echo "Environment=\"HTTP_PROXY=${HTTP_PROXY}\"" >> "${CONTAINERD_HTTP_PROXY}"
  need_restart=true
fi

if [[ "${HTTPS_PROXY}" != "" ]]; then
  echo "Environment=\"https_proxy=${HTTPS_PROXY}\"" >> "${CONTAINERD_HTTP_PROXY}"
  echo "Environment=\"HTTPS_PROXY=${HTTPS_PROXY}\"" >> "${CONTAINERD_HTTP_PROXY}"
  need_restart=true
fi

if [[ "${NO_PROXY}" != "" ]]; then
  echo "Environment=\"no_proxy=${NO_PROXY}\"" >> "${CONTAINERD_HTTP_PROXY}"
  echo "Environment=\"NO_PROXY=${NO_PROXY}\"" >> "${CONTAINERD_HTTP_PROXY}"
  need_restart=true
fi

if [[ "$need_restart" = "true" ]]; then
  snap restart k8s.containerd
fi
