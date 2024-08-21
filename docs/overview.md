# Cluster Provisioning with CAPI

This guide covers how to deploy a Canonical Kubernetes multi-node cluster using Cluster API (CAPI).

## Install `clusterctl`

The `clusterctl` CLI tool manages the lifecycle of a Cluster API management cluster. To install it, follow the [upstream instructions]. Typically, this involves fetching the executable that matches your hardware architecture and placing it in your PATH. For example, at the time this guide was written, for `amd64` you would:

```sh
curl -L https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.7.3/clusterctl-linux-amd64 -o clusterctl
sudo install -o root -g root -m 0755 clusterctl /usr/local/bin/clusterctl
```

### Set up a management Cluster

The management cluster hosts the CAPI providers. You can use a Canonical Kubernetes cluster as a management cluster:

```sh
sudo snap install k8s --classic --edge
sudo k8s bootstrap
sudo k8s status --wait-ready
mkdir -p ~/.kube/
sudo k8s kubectl config view --raw > ~/.kube/config
```

When setting up the management cluster, place its kubeconfig under `~/.kube/config` so other tools such as `clusterctl` can discover and interact with it.

### Prepare the Infrastructure Provider

Before generating a cluster, you need to configure the infrastructure provider. Each provider has its own prerequisites. Please follow the Cluster API instructions for the additional infrastructure-specific configuration.

#### Example Using AWS

The AWS infrastructure provider requires the `clusterawsadm` tool to be installed:

```sh
curl -L https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases/download/v2.0.2/clusterawsadm-linux-amd64 -o clusterawsadm
chmod +x clusterawsadm
sudo mv clusterawsadm /usr/local/bin
```

With `clusterawsadm`, you can bootstrap the AWS environment that CAPI will use.

Start by setting up environment variables defining the AWS account to use, if these are not already defined:

```sh
export AWS_REGION=<your-region-eg-us-east-2>
export AWS_ACCESS_KEY_ID=<your-access-key>
export AWS_SECRET_ACCESS_KEY=<your-secret-access-key>
```

If you are using multi-factor authentication, you will also need:

```sh
export AWS_SESSION_TOKEN=<session-token> # If you are using Multi-Factor Auth.
```

The `clusterawsadm` uses these details to create a CloudFormation stack in your AWS account with the correct IAM resources:

```sh
clusterawsadm bootstrap iam create-cloudformation-stack
```

The credentials should also be encoded and stored as a Kubernetes secret:

```sh
export AWS_B64ENCODED_CREDENTIALS=$(clusterawsadm bootstrap credentials encode-as-profile)
```

### Initialize the Management Cluster

To initialize the management cluster with the latest released version of the providers and the infrastructure of your choice:

```sh
clusterctl init --bootstrap ck8s --control-plane ck8s -i <infra-provider-of-choice>
```

### Generate a Cluster Spec Manifest

Once the bootstrap and control-plane controllers are up and running, you can apply the cluster manifests with the specifications of the cluster you want to provision.

You can generate a cluster manifest for an infrastructure using templates provided by the Canonical Kubernetes team. The templates/ folder contains templates for common clouds.

Ensure you have initialized the desired infrastructure provider and fetch the Canonical Kubernetes bootstrap provider repository.

Review the list of variables needed for the cluster template:

```sh
cd templates/<infra-provider-of-choice>
clusterctl generate cluster k8s-<cloud_provider> --from ./templates/cluster-template-<cloud_provider>.yaml --list-variables
```

Set the respective environment variables by editing the rc file as needed before sourcing it. Then generate the cluster manifest:

```sh
source ./templates/cluster-template-<cloud_provider>.rc
clusterctl generate cluster k8s-<cloud_provider> --from ./templates/cluster-template-<cloud_provider>.yaml > cluster.yaml
```

Each provisioned node is associated with a `K8sConfig`, through which you can set the cluster’s properties. Review the available options in the respective definitions file and edit the cluster manifest (`cluster.yaml` above) to match your needs. Note that the configuration structure is similar to that of `kubeadm` - in the `CK8sConfig`, you will find a `ClusterConfiguration` and an `InitConfiguration` section.

### Deploy the Cluster

To deploy the cluster, run:

```sh
sudo k8s kubectl apply -f cluster.yaml
```

To see the deployed machines:

```sh
sudo k8s kubectl get machine
```

After the first control plane node is provisioned, you can get the kubeconfig of the workload cluster:

```sh
clusterctl get kubeconfig <provisioned-cluster> > kubeconfig
```

You can then see the workload nodes using:

```sh
KUBECONFIG=./kubeconfig kubectl get node
```

### Delete the Cluster

To get the list of provisioned clusters:

```sh
sudo k8s kubectl get clusters
```

To delete a cluster:

```sh
sudo k8s kubectl delete cluster <provisioned-cluster>
```

<!-- Links -->
[upstream instructions]: https://cluster-api.sigs.k8s.io/user/quick-start#install-clusterctl
