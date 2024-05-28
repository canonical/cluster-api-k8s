# Cluster API Canonical Kubernetes

This repository contains bootstrap and control plane providers to deploy Canonical Kubernetes clusters using [Cluster API](https://github.com/kubernetes-sigs/cluster-api/blob/master/README.md).

CABPCK (Cluster API bootstrap provider for Canonical Kubernetes) is responsible for generate cloud-init scripts for generate Machines such that they run Kubernetes nodes. This implementation uses [Canonical Kubernetes](https://github.com/canonical/k8s-snap) to deliver Kubernetes.

CACPCK (Cluster API control plane provider for Canonical Kubernetes) is responsible for managing the lifecycle of machines that host the control plane nodes of a Canonical Kubernetes cluster.
