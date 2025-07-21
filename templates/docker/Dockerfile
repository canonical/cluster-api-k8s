####################################################################################################
####
#### k8s-snap Dockerfile
####
##
## # Example usage
##
## $ docker build -t ghcr.io/canonical/cluster-api-k8s/k8s-snap:v1.32.2
## $ docker build -t ghcr.io/canonical/cluster-api-k8s/k8s-snap:v1.32.2-moonray --build-arg BRANCH=autoupdate/moonray
##
## # Notes
##
## All decisions for this Dockerfile are documented in NOTE comments below.
## Things that need to be kept up to date for future versions are marked with !!!IMPORTANT!!!

####################################################################################################
## BASE IMAGES

## NOTE(neoaggelos): kindest/node base comes with systemd, important mounts, cgroups out of the box.
## See https://kind.sigs.k8s.io/docs/design/node-image/
##
## !!!IMPORTANT!!! Keep this up to date with the minor version of k8s-snap.
ARG BASE=kindest/node:v1.32.2

## NOTE(neoaggelos): Builder uses the ubuntu:20.04 as base image
##
## !!!IMPORTANT!!! Keep the base image up to date with the base image from "snapcraft.yaml:base".
ARG BUILD_BASE=ubuntu:20.04

####################################################################################################
## IMAGE(builder): Used as base image for building k8s-snap components

FROM $BUILD_BASE AS builder

## NOTE(neoaggelos): k8s-snap repo and branch to use for building
ARG REPO=https://github.com/canonical/k8s-snap
ARG BRANCH=main

ARG KUBERNETES_VERSION=""
# This is used to build the kubernetes version to upgrade when testing in place upgrades
ARG KUBERNETES_VERSION_UPGRADE_TO=""

## NOTE(neoaggelos): install dependencies needed to build the tools
## !!!IMPORTANT!!! Keep up to date with "snapcraft.yaml:parts.build-deps.build-packages"
RUN apt-get update \
    && apt-get install -y wget ca-certificates python3 sudo autoconf automake autopoint autotools-dev bison btrfs-progs libbtrfs-dev build-essential curl flex git libjansson-dev liblz4-dev libnetfilter-conntrack-dev libnetfilter-conntrack3 libnfnetlink-dev libseccomp-dev libtool pkg-config rsync --no-install-recommends

## NOTE(neoaggelos): Go version used to build components
## !!!IMPORTANT!!! Keep up to date with "snapcraft.yaml:parts.build-deps.build-snaps.go"
ADD install-go.sh /
RUN /install-go.sh 1.24.0 && ln -s /usr/local/go/bin/go /usr/local/bin/go

## Prepare build environment
ENV SNAPCRAFT_PART_INSTALL=/out
RUN mkdir /out /build -p
RUN git clone ${REPO} /src/k8s-snap -b ${BRANCH}

####################################################################################################
## IMAGE(builder-dqlite): Used as base image for building components that need dqlite

FROM builder AS builder-dqlite
RUN cp -rv /src/k8s-snap/src/k8s/hack /build/static-dqlite-deps
RUN bash -x /build/static-dqlite-deps/static-dqlite.sh
ENV DQLITE_BUILD_SCRIPTS_DIR=/build/static-dqlite-deps

####################################################################################################
## IMAGE(build-*): Build components

## k8sd build
FROM builder-dqlite AS build-k8sd
WORKDIR /src/k8s-snap/src/k8s
RUN make static -j
RUN mkdir -p /out/bin && mv ./bin/static/* /out/bin/

## components build
FROM builder-dqlite AS build-components
WORKDIR /src/k8s-snap
RUN ./build-scripts/build-component.sh k8s-dqlite
RUN ./build-scripts/build-component.sh cni
RUN ./build-scripts/build-component.sh etcd
RUN ./build-scripts/build-component.sh containerd
RUN ./build-scripts/build-component.sh helm
RUN ./build-scripts/build-component.sh runc
RUN ./build-scripts/build-component.sh pebble

# build kubernetes
ENV KUBERNETES_VERSION=${KUBERNETES_VERSION}
RUN if [ -n "$KUBERNETES_VERSION" ]; then \
      echo "Overwriting Kubernetes version with $KUBERNETES_VERSION"; \
      echo "$KUBERNETES_VERSION" > /src/k8s-snap/build-scripts/components/kubernetes/version; \
    fi
RUN  ./build-scripts/build-component.sh kubernetes

## generate bom.json file
RUN apt-get install -y python3-yaml
RUN bash -c "./build-scripts/generate-bom.py > /bom.json"

## kubernetes upgrade version build
FROM builder AS build-kubernetes-upgrade-to
ENV KUBERNETES_VERSION_UPGRADE_TO=${KUBERNETES_VERSION_UPGRADE_TO}
RUN if [ -n "$KUBERNETES_VERSION_UPGRADE_TO" ]; then \
      echo "Overwriting Kubernetes version with $KUBERNETES_VERSION_UPGRADE_TO"; \
      echo "$KUBERNETES_VERSION_UPGRADE_TO" > /src/k8s-snap/build-scripts/components/kubernetes/version; \
    fi
RUN  /src/k8s-snap/build-scripts/build-component.sh kubernetes

####################################################################################################
## IMAGE(build-preload-images): Fetch OCI images that can be pre-loaded to containerd

FROM build-k8sd AS build-preload-images
WORKDIR /src/k8s-snap/src/k8s/tools
RUN mkdir /out/images -p

## NOTE(neoaggelos): example invocation
## - regctl image export --platform local --user-agent containerd/v1.6.33 ghcr.io/canonical/pause:3.10 /out/images/ghcr.io-canonical-pause-3.10.tar
RUN /out/bin/k8s list-images \
    | xargs -n1 bash -c './regctl.sh image export \
        --platform local \
        --user-agent containerd/v1.6.33 \
        $0 \
        /out/images/$(echo $0 | tr ":/" "-").tar \
        || true'

####################################################################################################
## IMAGE(k8s-snap): k8s-snap image

FROM $BASE

## NOTE(neoaggelos): Initialize base image
## - Create directories for k8s-snap binaries and configs
## - Create directories for pebble
## - Disable the built-in containerd service
## - Nullify the crictl binary, because CAPD does a "crictl ps" before proceeding with bootstrap
RUN mkdir -p /var/lib/pebble/default/layers /snap/k8s/current /var/snap/k8s/common \
    && rm /etc/systemd/system/containerd.service \
    && ln -sf "$(which true)" "$(which crictl)"

## NOTE(neoaggelos): Install binaries
COPY --from=build-components /out /snap/k8s/current
COPY --from=build-components /bom.json /snap/k8s/current/
COPY --from=build-kubernetes-upgrade-to /out /k8s/upgrade
COPY --from=build-k8sd /out /snap/k8s/current
COPY --from=build-preload-images /out/images /var/snap/k8s/common/images

## NOTE(Reza): Install shared objects
COPY --from=build-components /build/static-dqlite-deps/.deps/dynamic/lib /usr/lib
COPY --from=build-components /build/static-dqlite-deps/.deps/dynamic/include /usr/include

## NOTE(neoaggelos): Install k8s files
COPY --from=builder /src/k8s-snap/k8s /snap/k8s/current/k8s

## NOTE(neoaggelos): Enable pebble and configure k8sd to use it for services
COPY --from=builder /src/k8s-snap/k8s/pebble/pebble.service /etc/systemd/system/pebble.service
COPY --from=builder /src/k8s-snap/k8s/pebble/000-k8s.yaml /var/lib/pebble/default/layers/000-k8s.yaml
RUN systemctl enable pebble
ENV K8SD_RUNTIME_ENVIRONMENT=pebble

## NOTE(neoaggelos): Setup runtime environment
ENV SNAP=/snap/k8s/current
ENV SNAP_REVISION=current
ENV SNAP_COMMON=/var/snap/k8s/common
ENV REAL_PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/snap/k8s/current/bin

## NOTE(neoaggelos): Required for containerd to properly set up overlayfs for pods
VOLUME ["/var/snap/k8s/common/var/lib/containerd"]

## NOTE(ben): Remove existing kind image kubectl and kubelet binaries
# to avoid version confusion.
RUN rm -f /usr/bin/kubectl /usr/bin/kubelet
