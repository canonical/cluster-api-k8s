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
| OPENSTACK_BASTION_ENABLED | Deploy an ssh bastion (jump server) instance that can be used to access the cluster nodes. |
| OPENSTACK_SSH_KEY_NAME | The keypair used to access Openstack instances. |
| OPENSTACK_IMAGE_NAME | The Openstack image used when deploying instances. |
| OPENSTACK_EXTERNAL_NETWORK_ID | The external Openstack network id. |
| OPENSTACK_CLOUD | The name of the cloud from the [clouds.yaml] file. |
| OPENSTACK_CLOUD_YAML_B64 | Base64 encoded [clouds.yaml] file, containing Openstack credentials. |
| OPENSTACK_CLOUD_CONFIG_B64 | Base64 encoded [Openstack Cloud Controller Manager] configuration. |
| OPENSTACK_CLOUD_CACERT_B64 | Optional: base64 encoded CA certificate used to contact Openstack services. |
| OPENSTACK_ENABLE_APISERVER_LOADBALANCER | Set to "true" to use Octavia load balancers. |
| OPENSTACK_FAILURE_DOMAIN | The OpenStack Nova availability zone. |
| OPENSTACK_CONTROL_PLANE_MACHINE_FLAVOR | The Opentack flavor used to deploy control plane nodes. |
| OPENSTACK_NODE_MACHINE_FLAVOR | The Opentack flavor used to deploy worker nodes. |
| OPENSTACK_BASTION_MACHINE_FLAVOR | The Openstack flavor used to deploy the bastion machine. |
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

We strongly recommend enabling Octavia (Load-Balancer-as-a-Service) as well:

```
cat <<EOF >> local.sh

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

### With Octavia LBaaS

The cluster should be ready in a few minutes. Using the default rc file, we'll
have the following instances:

* control plane node
* worker node
* two Octavia "amphora" instances, one for each Openstack load balancer:
    * one load balancer handling kube-apiserver traffic
    * another load balancer for Kubernetes ingress

```
openstack server list

# output
+--------------------------------------+----------------------------------------------+--------+------------------------------------------------------------------------+---------------------+------------+
| ID                                   | Name                                         | Status | Networks                                                               | Image               | Flavor     |
+--------------------------------------+----------------------------------------------+--------+------------------------------------------------------------------------+---------------------+------------+
| 2e57a19c-ed7f-48bc-a611-a630d28c0fcc | c1-md-0-g6nrp-rwghf                          | ACTIVE | k8s-clusterapi-cluster-default-c1=10.6.0.31                            | ubuntu-noble        | ds4G       |
| bd1e1c5d-bb56-410d-82cb-d3b79e9df5f3 | amphora-03efb47b-21ac-4f18-b6c7-bf83d01beef3 | ACTIVE | k8s-clusterapi-cluster-default-c1=10.6.0.225; lb-mgmt-net=192.168.0.39 | amphora-x64-haproxy | m1.amphora |
| b5aad429-6799-42f8-a7ae-c49662b4f1f7 | c1-control-plane-x29bs                       | ACTIVE | k8s-clusterapi-cluster-default-c1=10.6.0.144                           | ubuntu-noble        | ds4G       |
| e00db5a7-53fa-4b00-93ff-af56a394f4ef | c1-bastion                                   | ACTIVE | k8s-clusterapi-cluster-default-c1=10.6.0.73, 172.24.4.49               | ubuntu-noble        | m1.small   |
| 68704dae-fa99-4b0f-a592-b8fd6326e78b | amphora-ac03b766-544e-43ea-90da-e7ef793d59a7 | ACTIVE | k8s-clusterapi-cluster-default-c1=10.6.0.27; lb-mgmt-net=192.168.0.53  | amphora-x64-haproxy | m1.amphora |
+--------------------------------------+----------------------------------------------+--------+------------------------------------------------------------------------+---------------------+------------+
```

Openstack instances normally use private networks and can be accessed
through floating IPs from public networks.

In our case, the Kubernetes API will be exposed through the floating ip
associated with the load balancer port.

For debugging purposes, we can also use the bastion (jump server) machine to
ssh into the Kubernetes nodes. This spares us from having to manually
set floating ips and security group rules, which could interfere with the
CAPO controller (e.g. prevent network deletion).

### Without Octavia

When Octavia is not available, the floating IPs are associated directly to
the Kubernetes machines. This prevents us from having more than one
control plane node and should only be used for testing purposes.

``kube-vip`` may be used as a workaround, however this setup is not officially
supported by the Openstack CAPI provider.

### Obtaining the workload cluster kubeconfig

Use the following to obtain a kubeconfig for the workload cluster:

```
clusterName="c1"
kubeconfig=/tmp/${clusterName}_kubeconfig
clusterctl get kubeconfig $clusterName > $kubeconfig
```

The kubeconfig uses a floating ip address, which can be attached to either the
load balancer port when using Octavia or the control plane node otherwise.

```
grep server $kubeconfig

# output
    server: https://172.24.4.199:6443
```

Use the kubeconfig node to check the workload cluster nodes:

```
KUBECONFIG=$kubeconfig kubectl get nodes
NAME                     STATUS   ROLES                  AGE   VERSION
c1-control-plane-x29bs   Ready    control-plane,worker   40m   v1.32.2
c1-md-0-g6nrp-rwghf      Ready    worker                 37m   v1.32.2.2
```

<!-- LINKS -->
[clouds.yaml]: https://docs.openstack.org/python-openstackclient/2024.2/configuration/index.html#clouds-yaml
[CAPO provider]: https://github.com/kubernetes-sigs/cluster-api-provider-openstack
[Openstack Cloud Controller Manager]: https://github.com/kubernetes/cloud-provider-openstack/blob/master/docs/openstack-cloud-controller-manager/using-openstack-cloud-controller-manager.md
[template-varaibles.rc]: ./template-variables.rc
[template-variables-devstack.rc]: ./template-variables-devstack.rc
[Devstack]: https://github.com/openstack/devstack
[Kind]: https://kind.sigs.k8s.io/docs/user/quick-start/#installation
[clusterctl]: https://cluster-api.sigs.k8s.io/user/quick-start.html#install-clusterctl
