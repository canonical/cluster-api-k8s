#!/bin/bash

set -x

services="containerd k8s-dqlite k8sd kube-apiserver kube-controller-manager kube-proxy kube-scheduler kubelet"

mkdir -p logs

until [ `docker ps -a | grep "control-plane-" | wc -l` = "6" ]; do
  echo "Waiting for control plane nodes...";
  sleep 60
done

# k8s services may still need to be initialized before we start collecting logs.
sleep 30

docker ps -a > docker-ps.txt

while read -r container; do
  container_name="${container##* }"
  mkdir logs/$container_name

  for service in $services; do
    nohup docker exec $container_name pebble logs -f -n 1000000 $service >> logs/$container_name/$service.log &
  done
done < <(docker ps -a | grep control-plane-)
