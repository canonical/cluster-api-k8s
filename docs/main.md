# CABPCK and CACPCK documentation

- CABPCK is the bootstrap provider, responsible for generate cloud-init scripts for machines.
- CACPCK is the control plane provider, responsible for the lifecycle of the control plane machines.

## Technical Topics

Below technical topics and design decisions for the providers are discussed.

### Snap installation

The default behaviour is to `snap install k8s` using the matching track (e.g. install version `1.30.1` will install from `--channel 1.30-classic`).

You can override this behaviour by changing the default installation script by setting the following fields on the config template:

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1beta1
kind: CK8sControlPlane
metadata:
  name: ${CLUSTER_NAME}-control-plane
  namespace: default
spec:
  spec:
    extraFiles:
      - path: /capi/scripts/install.sh
        permissions: "0500"
        owner: "root:root"
        content: |
          #!/bin/bash -xe

          # # install specific revision
          # snap install k8s --classic --revision $revision

          # OR from local file, assuming the use of a custom image
          # snap install --classic --dangerous /opt/k8s.snap

          # OR do nothing, assuming snap is pre-installed on a custom image
```

For airgap deployments, or environment you can specify `airGapped: true` to prevent the install script from running. In such setups, one of the things below are expected:

- The user specifies `preRunCommands` that install `k8s-snap` manually.
- The user provides a custom OS image for their cloud with `k8s-snap` pre-installed.

### Deploy manifests

Any extra yaml files placed in `/capi/manifests` will be applied once on the cluster after bootstrapping. Files are applied in alphabetical order, so you can use this in case of dependencies. Example:

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1beta1
kind: CK8sControlPlane
metadata:
  name: ${CLUSTER_NAME}-control-plane
  namespace: default
spec:
  spec:
    files:
      - path: /capi/manifests/00-cm.yaml
        mode: "0400"
        owner: "root:root"
        content: |
          apiVersion: v1
          kind: ConfigMap
          metadata:
            name: test-cm
            namespace: kube-system
          data:
            key: value
```

### k8sd proxy

A `k8sd-proxy` daemonset is deployed on the cluster. A pod is running on each cluster node, listening on port 2380 and forwarding traffic to the node's 2380 port (or whatever port k8sd is listening on). This allows to use the `client-go` and the kubeconfig of the workload cluster to reach the k8sd service on any of the cluster nodes.

The main uses of this k8sd-proxy are:
- Generate tokens for joining more cluster nodes
- (Possibly, in the future) Use the k8sd API to manage the cluster (by issuing `k8s set` calls through the API and managing the cluster configuration).
- (Possibly, in the future) Perform in-place certificate rotations
- (Possibly, in the future) Perform in-place cluster upgrades

We reach the k8sd service by using client-go to list the `k8sd-proxy` daemonset pods, locating the pod that is on our target node, then using the `pods/proxy` or `pods/portforward` subresource (specifics tbd during implementation).

To authenticate with k8sd, we use the pre-shared token (specifics tbd during implementation).

As for the implementation, we currently go with option 1 for simplicity, but option 2 should be considered for the future:

1. A daemonset with a pod running on each node. This pod runs a `alpine/socat` and runs a tcp forward towards the k8sd port running on the node IP.
2. A deployment running on any node. k8sd manages a secret/configmap resource on the cluster with the node addresses and fingerprints. Each pod listens on a range of ports e.g. (10000 - 10250) and maps individual nodes to separate ports.

### Join Tokens

When bootstrapping, a `$cluster-token` is created on the control cluster. This is a master token set once when bootstrapping the cluster, and is used to authenticate requests from the providers.

When reconciling the cloud-init data of a joining control plane or worker node, the bootstrap provider shall use `k8sd-proxy` to find any active control plane nodes, and reach any of them to create a new join token for the target node. The master token is used to authenticate that request.

> *NOTE*: In the future, x509 auth might be used instead, but microcluster does not currently allow a whitelist of client certificates not tied to a microcluster node.

After generating the join token, it is seeded in the cloud-init data of the instance, and the instance uses it to join the cluster.
