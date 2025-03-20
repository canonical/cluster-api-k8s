# About

This template can be used to deploy Canonical Kubernetes on OpenStack using
the [CAPO provider].

Unlike the templates from the CAPO repository, it uses the Canonical Kubernetes
providers instead of Kubeadm.

# Configuration

The templates use the following variables:

| Variable | Description |
|-|-|
| KUBERNETES_VERSION | The Kubernetes version of the cluster, e.g. "v1.32.2". |
| CONTROL_PLANE_MACHINE_COUNT | The number of control plane nodes. |
| WORKER_MACHINE_COUNT | The number of worker nodes. |
| OPENSTACK_SSH_KEY_NAME | The keypair used to access Openstack instances. |
| OPENSTACK_IMAGE_NAME | The Openstack image used when deploying Canonical K8s nodes. |
| OPENSTACK_EXTERNAL_NETWORK_ID | The external Openstack network id. |
| OPENSTACK_CLOUD | The name of the cloud from the [clouds.yaml] file. |
| OPENSTACK_CLOUD_YAML_B64 | Base64 encoded [clouds.yaml] file, containing Openstack credentials. |
| OPENSTACK_CLOUD_CONFIG_B64 | Base64 encoded [Openstack Cloud Controller Manager] configuration. |
| OPENSTACK_CLOUD_CACERT_B64 | Optional: base64 encoded CA certificate used to contact Openstack services. |
| OPENSTACK_ENABLE_APISERVER_LOADBALANCER | Set to "true" to use Octavia load balancers. |
| OPENSTACK_FAILURE_DOMAIN | The OpenStack Nova availability zone. |
| OPENSTACK_CONTROL_PLANE_MACHINE_FLAVOR | The Opentack flavor used to deploy control plane nodes. |
| OPENSTACK_NODE_MACHINE_FLAVOR | The Opentack flavor used to deploy worker nodes. |
| OPENSTACK_DNS_NAMESERVERS | The list of DNS nameservers to use for Openstack instances. |

Feel free to use the [template-varaibles.rc] template to define the environment variables or
[template-variables-devstack.rc] for [Devstack] environments.

# Devstack

[Devstack] can be used to quickly set up an Openstack environment for testing or
development purposes. We recommend using a clean virtual machine with nested virtualization
enabled.

Start by cloning the devstack repository:

```
git clone https://github.com/openstack/devstack
```

Prepare the Devstack configuration file:

```
cd devstack

cat <<EOF > local.sh
[[local|localrc]]
ADMIN_PASSWORD=secret
DATABASE_PASSWORD=\$ADMIN_PASSWORD
RABBIT_PASSWORD=\$ADMIN_PASSWORD
SERVICE_PASSWORD=\$ADMIN_PASSWORD

IP_VERSION=4
EOF
```

Octavia (Load-Balancer-as-a-Service) can be enabled like so:

```
cat <<EOF >> local.sh

IP_VERSION=4

# Octavia (LoadBalancer-as-a-Service) settings.
GIT_BASE=https://opendev.org
enable_plugin neutron $GIT_BASE/openstack/neutron
enable_plugin octavia \$GIT_BASE/openstack/octavia master
enable_plugin ovn-octavia-provider \$GIT_BASE/openstack/ovn-octavia-provider
enable_service octavia o-api o-cw o-hm o-hk o-da
```

Run ``./stack.sh`` to initialize the OpenStack environment using Devstack.

## CAPI prerequisites

Start by installing [Kind] and [clusterctl].

The CAPO provider relies on the Openstack Resource Controller, which can be
installed like so:

```
kubectl apply -f https://github.com/k-orc/openstack-resource-controller/releases/latest/download/install.yaml
```

Initialize the CAPI providers:

```
clusterctl init -i openstack -b canonical-kubernetes -c canonical-kubernetes
```

## Define the template variables

For Devstack environments, [template-variables-devstack.rc] can be used to
automatically set all the variables required by the Openstack template.

First, make sure to source the Devstack ``openrc`` file. For testing purposes,
we are going to use admin credentials.

```
source $devstackDir/openrc admin admin
```

By default, it will create an Openstack image using Ubuntu 24.04 (noble).
It also creates an Openstack keypair, expecting a ssh public key at ``~/.ssh/id_rsa.pub``,
use the ``$SSH_PUBKEY`` variable to specify a different path.

The script checks if Octavia is enabled and sets the corresponding template variables.

## Create the cluster

Source [template-variables-devstack.rc]:

```
source ./templates/openstack/template-variables-devstack.rc
```

Generate the manifests:

```
clusterctl generate cluster c1 --from ./templates/openstack/cluster-template.yaml  > c1.yaml
```

Create the workload cluster:

```
kubectl apply -f c1.yaml
```

## Verifying the deployment

The control plane instance should become active in a few minutes and be accessible through its
floating ip address.

```
openstack server list

# output
+--------------------------------------+------------------------+--------+------------------------------------------------------------+--------------+-----------+
| ID                                   | Name                   | Status | Networks                                                   | Image        | Flavor    |
+--------------------------------------+------------------------+--------+------------------------------------------------------------+--------------+-----------+
| a51e5053-59bd-4da3-98c9-479858a84b59 | c1-control-plane-9qtfm | ACTIVE | k8s-clusterapi-cluster-default-c1=10.6.0.123, 172.24.4.123 | ubuntu-noble | m1.medium |
+--------------------------------------+------------------------+--------+------------------------------------------------------------+--------------+-----------+
```

We'll create and assign a new security group to allow SSH traffic:

```
cpNode=$(kubectl get machine -o "jsonpath={.items[0].status.nodeRef.name}"  | grep control-plane)
openstack security group create ssh 
openstack security group rule create ssh --protocol tcp --dst-port 22
openstack server add security group $cpNode ssh
```

Obtain the floating IP address and ssh into the control plane node:

```
cpFip=$(openstack server show  $cpNode -f json | jq '.addresses[][1]' -r)
ssh ubuntu@$cpFip
```

Verify the node status:

```
sudo k8s kubectl get nodes
NAME                     STATUS   ROLES                  AGE   VERSION
c1-control-plane-9qtfm   Ready    control-plane,worker   68m   v1.32.2
```

<!-- LINKS -->
[clouds.yaml]: https://docs.openstack.org/python-openstackclient/2024.2/configuration/index.html#clouds-yaml
[CAPO provider]: https://github.com/kubernetes-sigs/cluster-api-provider-openstack
[Openstack Cloud Controller Manager]: https://github.com/kubernetes/cloud-provider-openstack/blob/master/docs/openstack-cloud-controller-manager/using-openstack-cloud-controller-manager.md
[template-varaibles.rc]: ./template-variables.rc
[template-varaibles-devstack.rc]: ./template-variables-devstack.rc
[Devstack]: https://github.com/openstack/devstack
[Kind]: https://kind.sigs.k8s.io/docs/user/quick-start/#installation
[clusterctl]: https://cluster-api.sigs.k8s.io/user/quick-start.html#install-clusterctl
