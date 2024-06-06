# Quickstart

Describes the steps to create and deploy the Canonical K8s Bootstrap and Control-plane providers on a Canonical K8s managment cluster with in-memory infrastructure provider.
This is absolutely not intended for any kind of production environment.

Note: This tutorial assumes that you have the repository at `~/cluster-api-k8s`

## Let's start

Setup inital cluster that will be transformed into managment cluster later.

```bash
sudo snap install k8s --edge --classic
sudo k8s bootstrap
mkdir ~/.kube
sudo k8s config > ~/.kube/config
```

Install clusterctl

```bash
sudo snap install clusterctl
```

Create the Canonical Kubernetes provider release:

```bash
cd ~/cluster-api-k8s
make release
```

This will create the necessary yaml files in `~/cluster-api-k8s/out`.
CAPI expects a very specific structure for the providers (`{basepath}/{provider-label}/{version}/{components.yaml}`). Let's reshuffle the files to match this pattern.

```bash
mkdir -p control-plane-canonical-k8s-provider/0.1.0
mkdir -p bootstrap-canonical-k8s-provider/0.1.0

cp ~/cluster-api-k8s/out/manifest.yaml ~/cluster-api-k8s/out/bootstrap-components.yaml ~/bootstrap-canonical-k8s-provider/0.1.0/
cp ~/cluster-api-k8s/out/manifest.yaml ~/cluster-api-k8s/out/control-plane-components.yaml ~/control-plane-canonical-k8s-provider/0.1.0/
```

We now need to tell `clusterctl` about this new providers.
Create a custom clusterctl config:

```bash
cat <<EOF > ~/clusterctl.yaml
providers:
  - name: "canonical-k8s-bootstrap-provider"
    url: "${HOME}/bootstrap-canonical-k8s-provider/0.1.0/bootstrap-components.yaml"
    type: "BootstrapProvider"
  - name: "canonical-k8s-control-plane-provider"
    url: "${HOME}/control-plane-canonical-k8s-provider/0.1.0/control-plane-components.yaml"
    type: "ControlPlaneProvider"
EOF
```

You can verify that clusterctl has picked up the providers with:

```bash
ubuntu@brisk-agouti:~$ clusterctl --config ~/clusterctl.yaml config repositories | grep canonical-k8s
canonical-k8s-bootstrap-provider       BootstrapProvider        /home/ubuntu/bootstrap-canonical-k8s-bootstrap-provider/latest/                             bootstrap-components.yaml
canonical-k8s-control-plane-provider   ControlPlaneProvider     /home/ubuntu/control-plane-canonical-k8s-control-plane-provider/latest/                     control-plane-components.yaml
```

Now, we can initialize the cluster, we omit the infrastructure provider for now and deploy it manually later:

```bash
ubuntu@brisk-agouti:~$ clusterctl --config ~/clusterctl.yaml init --infrastructure - --bootstrap canonical-k8s-bootstrap-provider --control-plane canonical-k8s-control-plane-provider
Fetching providers
Installing cert-manager Version="v1.14.2"
Waiting for cert-manager to be available...
Installing Provider="cluster-api" Version="v1.7.2" TargetNamespace="capi-system"
Installing Provider="bootstrap-canonical-k8s-bootstrap-provider" Version="0.1.0" TargetNamespace="cabpck-system"
Installing Provider="control-plane-canonical-k8s-control-plane-provider" Version="0.1.0" TargetNamespace="cacpck-system"

Your management cluster has been initialized successfully!

You can now create your first workload cluster by running the following:

  clusterctl generate cluster [name] --kubernetes-version [version] | kubectl apply -f -
```

Let's deploy the in-memory (fake) infrastructure provider:

```bash
ubuntu@brisk-agouti:~$ sudo k8s kubectl apply -f "https://github.com/neoaggelos/cluster-api-provider-inmemory-microk8s/releases/download/20240410-dev1/infrastructure-components-in-memory-development.yaml"
namespace/capim-system created
customresourcedefinition.apiextensions.k8s.io/inmemoryclusters.infrastructure.cluster.x-k8s.io created
customresourcedefinition.apiextensions.k8s.io/inmemoryclustertemplates.infrastructure.cluster.x-k8s.io created
customresourcedefinition.apiextensions.k8s.io/inmemorymachines.infrastructure.cluster.x-k8s.io created
customresourcedefinition.apiextensions.k8s.io/inmemorymachinetemplates.infrastructure.cluster.x-k8s.io created
serviceaccount/capim-manager created
role.rbac.authorization.k8s.io/capim-leader-election-role created
clusterrole.rbac.authorization.k8s.io/capim-manager-role created
rolebinding.rbac.authorization.k8s.io/capim-leader-election-rolebinding created
clusterrolebinding.rbac.authorization.k8s.io/capim-manager-rolebinding created
service/capim-webhook-service created
deployment.apps/capim-controller-manager created
certificate.cert-manager.io/capim-serving-cert created
issuer.cert-manager.io/capim-selfsigned-issuer created
mutatingwebhookconfiguration.admissionregistration.k8s.io/capim-mutating-webhook-configuration created
validatingwebhookconfiguration.admissionregistration.k8s.io/capim-validating-webhook-configuration created
```

Download the cluster-template:

```bash
curl -fsSL "https://github.com/neoaggelos/cluster-api-provider-inmemory-microk8s/releases/download/20240410-dev1/cluster-template.yaml" -o cluster-template.yaml
```

Replace the `MicroK8s` occurences with `CK8s`

```bash
sed "s/MicroK8s/CK8s/g" ~/cluster-template.yaml > ~/cluster-template.yaml
```

Now, we can generate a cluster config:

```bash
export CONTROL_PLANE_MACHINE_COUNT=1
export WORKER_MACHINE_COUNT=0
export KUBERNETES_VERSION="1.30.0"

clusterctl generate cluster "my-cluster" --from ./cluster-template.yaml > "my-cluster.yaml"
```

And finally, apply it:

```bash
sudo k8s kubectl apply -f ~/my-cluster.yaml
```
