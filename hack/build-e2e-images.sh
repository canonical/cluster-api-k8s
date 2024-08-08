cd ../templates/docker
sudo docker build  -t k8s-snap:dev-old --build-arg BRANCH=main --build-arg KUBERNETES_VERSION=v1.29.6
sudo docker build . -t k8s-snap:dev-new --build-arg BRANCH=main
