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
    sudo docker build . -t k8s-snap:dev --build-arg BRANCH=main
)

# create network 'kind', required by capd
sudo docker network create kind --driver=bridge -o com.docker.network.bridge.enable_ip_masquerade=true
```

### Create management cluster

```bash
# mount /var/run/docker.sock inside the container, so that we can create docker containers for workload clusters
sudo docker run --name management-cluster --network kind --rm --detach --privileged -v /var/run/docker.sock:/docker.sock k8s-snap:dev

# mounting directly at /var/run/docker.sock does not work, as systemd (?) overrides the path
sudo docker exec management-cluster ln -s /docker.sock /var/run/docker.sock

# bootstrap management cluster
sudo docker exec management-cluster k8s bootstrap

# get kubeconfig
mkdir -p ~/.kube
sudo docker exec management-cluster k8s config > ~/.kube/config
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
clusterctl generate cluster c1 --from ./templates/docker/cluster-template.yaml --list-variables

# configure image and generate cluster
export KIND_IMAGE=k8s-snap:dev
clusterctl generate cluster c1 --from ./templates/docker/cluster-template.yaml --kubernetes-version v1.30.1 > c1.yaml

# create cluster
kubectl create -f c1.yaml
```

### Check status

```bash
sudo docker ps                                                  # check running containers
kubectl get cluster,machine,ck8scontrolplane,secrets            # get overview of workload cluster resources
clusterctl describe cluster c1                                  # describe the cluster
clusterctl get kubeconfig c1 > kubeconfig                       # get the workload cluster kubeconfig file
kubectl get pod,node -A -o wide --kubeconfig=kubeconfig         # interact with the workload cluster
```

##### Example outputs

```bash
# check running containers
$ sudo docker ps
CONTAINER ID   IMAGE                                COMMAND                  CREATED          STATUS          PORTS                              NAMES
7a46ef0c1c92   k8s-snap:dev                         "/usr/local/bin/entr…"   7 minutes ago    Up 7 minutes    0/tcp, 127.0.0.1:32771->6443/tcp   c1-control-plane-wkqjc
1bd9fffbc5f9   kindest/haproxy:v20230510-486859a6   "haproxy -W -db -f /…"   7 minutes ago    Up 7 minutes    0/tcp, 0.0.0.0:32770->6443/tcp     c1-lb
3812c4ee555f   k8s-snap:dev                         "/usr/local/bin/entr…"   13 minutes ago   Up 13 minutes                                      management-cluster

# get overview of workload cluster resources
$ kubectl get cluster,machine,ck8scontrolplane,secrets
NAME                          CLUSTERCLASS   PHASE         AGE    VERSION
cluster.cluster.x-k8s.io/c1                  Provisioned   7m5s

NAME                                              CLUSTER   NODENAME                 PROVIDERID                          PHASE     AGE    VERSION
machine.cluster.x-k8s.io/c1-control-plane-wkqjc   c1        c1-control-plane-wkqjc   docker:////c1-control-plane-wkqjc   Running   7m3s   v1.30.1

NAME                                                              INITIALIZED   API SERVER AVAILABLE   VERSION   REPLICAS   READY   UPDATED   UNAVAILABLE
ck8scontrolplane.controlplane.cluster.x-k8s.io/c1-control-plane   true          true                   v1.30.1   1          1       1

NAME                            TYPE                      DATA   AGE
secret/c1-ca                    cluster.x-k8s.io/secret   2      7m3s
secret/c1-cca                   cluster.x-k8s.io/secret   2      7m3s
secret/c1-control-plane-n5lbj   cluster.x-k8s.io/secret   1      7m2s
secret/c1-kubeconfig            Opaque                    1      7m3s
secret/c1-token                 cluster.x-k8s.io/secret   1      7m3s

# describe the cluster
$ clusterctl describe cluster c1
NAME                                                READY  SEVERITY  REASON  SINCE  MESSAGE
Cluster/c1                                          True                     6m14s
├─ClusterInfrastructure - DockerCluster/c1          True                     7m5s
└─ControlPlane - CK8sControlPlane/c1-control-plane  True                     6m14s
  └─Machine/c1-control-plane-wkqjc                  True                     6m41s

# get the workload cluster kubeconfig file
$ clusterctl get kubeconfig c1 > kubeconfig

# interact with the workload cluster
$ kubectl get pod,node -A -o wide --kubeconfig=kubeconfig
NAMESPACE          NAME                                        READY   STATUS    RESTARTS   AGE     IP           NODE                     NOMINATED NODE   READINESS GATES
calico-apiserver   pod/calico-apiserver-6cfcfdbd65-5strm       1/1     Running   0          5m49s   10.1.220.8   c1-control-plane-wkqjc   <none>           <none>
calico-apiserver   pod/calico-apiserver-6cfcfdbd65-bwqq4       1/1     Running   0          5m49s   10.1.220.7   c1-control-plane-wkqjc   <none>           <none>
calico-system      pod/calico-kube-controllers-fbc894f-kkvqz   1/1     Running   0          6m28s   10.1.220.5   c1-control-plane-wkqjc   <none>           <none>
calico-system      pod/calico-node-v5wkn                       1/1     Running   0          6m23s   172.19.0.4   c1-control-plane-wkqjc   <none>           <none>
calico-system      pod/calico-typha-ccccd9786-tzlbk            1/1     Running   0          6m28s   172.19.0.4   c1-control-plane-wkqjc   <none>           <none>
calico-system      pod/csi-node-driver-bzlhd                   2/2     Running   0          6m29s   10.1.220.4   c1-control-plane-wkqjc   <none>           <none>
kube-system        pod/ck-storage-rawfile-csi-controller-0     2/2     Running   0          6m39s   10.1.220.2   c1-control-plane-wkqjc   <none>           <none>
kube-system        pod/ck-storage-rawfile-csi-node-vhcgz       4/4     Running   0          6m38s   10.1.220.1   c1-control-plane-wkqjc   <none>           <none>
kube-system        pod/coredns-7d4dffcffd-chtnq                1/1     Running   0          6m38s   10.1.220.3   c1-control-plane-wkqjc   <none>           <none>
kube-system        pod/metrics-server-6f66c6cc48-mv2vg         1/1     Running   0          6m38s   10.1.220.6   c1-control-plane-wkqjc   <none>           <none>
tigera-operator    pod/tigera-operator-76ff79f7fd-5k8jn        1/1     Running   0          6m38s   172.19.0.4   c1-control-plane-wkqjc   <none>           <none>

NAMESPACE   NAME                          STATUS   ROLES                  AGE     VERSION   INTERNAL-IP   EXTERNAL-IP   OS-IMAGE                         KERNEL-VERSION      CONTAINER-RUNTIME
            node/c1-control-plane-wkqjc   Ready    control-plane,worker   6m45s   v1.30.1   172.19.0.4    <none>        Debian GNU/Linux 12 (bookworm)   5.4.0-182-generic   containerd://1.6.33
```

### Delete workload cluster

```bash
kubectl delete cluster c1
```

### Delete management cluster

```bash
sudo docker rm -fv management-cluster
```
