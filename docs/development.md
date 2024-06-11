## Develop with Docker

Install docker, kubectl and clusterctl

```bash
curl get.docker.io -fsSL | bash
sudo snap install kubectl --classic
sudo snap install clusterctl --devmode --edge
```

### Prepare

```bash
# build docker image for k8s-snap
(
    cd templates/docker
    docker build . -t ghcr.io/canonical/cluster-api-k8s/k8s-snap:v1.30.1 --build-arg BRANCH=autoupdate/moonray
)

# create network 'kind', required by capd
sudo docker network create kind --driver=bridge -o com.docker.network.bridge.enable_ip_masquerade=true
```

### Create management cluster

```bash
# mount /var/run/docker.sock inside the container, so that we can create docker containers for workload clusters
sudo docker run --name management-cluster --network kind --rm --detach --privileged -v /var/run/docker.sock:/docker.sock ghcr.io/canonical/cluster-api-k8s/k8s-snap:v1.30.1

# TODO(neoaggelos): dev
sudo docker run --name management-cluster --network kind --rm --detach --privileged -v ~/hosts.d:/var/snap/k8s/common/etc/containerd/hosts.d:ro -v /var/run/docker.sock:/docker.sock ghcr.io/canonical/cluster-api-k8s/k8s-snap:v1.30.1

# mounting directly at /var/run/docker.sock does not work, as systemd (?) overrides the path
sudo docker exec management-cluster ln -s /docker.sock /var/run/docker.sock

# bootstrap management cluster
sudo docker exec management-cluster k8s bootstrap

# get kubeconfig
mkdir -p ~/.kube
docker exec management-cluster k8s config > ~/.kube/config
```

### Initialize ClusterAPI

```bash
# export GOPROXY=off                    # in case of failures with GitHub
clusterctl init -i docker -b - -c -
```

### Install CRDs

<!-- TODO(neoaggelos): build provider images and properly install on the cluster -->

```bash
# install CRDs
make install

# run bootstrap provider
make dev-bootstrap &

# run control-plane provider
make dev-controlplane &
```

### Deploy cluster

```bash
# check cluster template variables
clusterctl generate cluster --from ./templates/docker/cluster-template.yaml --list-variables

# configure image and generate cluster
export KIND_IMAGE=ghcr.io/canonical/cluster-api-k8s/k8s-snap:v1.30.1
clusterctl generate cluster c1 --from ./templates/docker/cluster-template.yaml --kubernetes-version v1.30.1 > c1.yaml

# create cluster
kubectl create -f c1.yaml
```

### Watch progress

```bash
# running
docker ps
