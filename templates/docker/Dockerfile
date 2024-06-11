####################################################################################################
####
#### k8s-snap Dockerfile
####
##
## # Example usage
##
## $ docker build -t ghcr.io/canonical/cluster-api-k8s/k8s-snap:v1.30.1
## $ docker build -t ghcr.io/canonical/cluster-api-k8s/k8s-snap:v1.30.1-moonray --build-arg BRANCH=autoupdate/moonray
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
ARG BASE=kindest/node:v1.30.0

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

## NOTE(neoaggelos): install dependencies needed to build the tools
## !!!IMPORTANT!!! Keep up to date with "snapcraft.yaml:parts.build-deps.build-packages"
RUN apt-get update \
    && apt-get install -y wget ca-certificates python3 sudo autoconf automake autopoint autotools-dev bison btrfs-progs libbtrfs-dev build-essential curl flex git libjansson-dev liblz4-dev libnetfilter-conntrack-dev libnetfilter-conntrack3 libnfnetlink-dev libseccomp-dev libtool pkg-config rsync --no-install-recommends

## NOTE(neoaggelos): Go version used to build components
## !!!IMPORTANT!!! Keep up to date with "snapcraft.yaml:parts.build-deps.build-snaps.go"
RUN wget https://go.dev/dl/go1.22.4.linux-amd64.tar.gz \
    && mkdir -p /usr/local/go122 \
    && tar -C /usr/local/go122 -xzf go1.22.4.linux-amd64.tar.gz \
    && ln -s /usr/local/go122/go/bin/go /usr/local/bin/go

## NOTE(neoaggelos): runc needs Go 1.21 to build
## !!!IMPORTANT!!! Drop this after runc 1.1.13 is released https://github.com/opencontainers/runc/issues/4233#issuecomment-2155883159
RUN wget https://go.dev/dl/go1.21.11.linux-amd64.tar.gz \
    && mkdir -p /usr/local/go121 \
    && tar -C /usr/local/go121 -xzf go1.21.11.linux-amd64.tar.gz \
    && ln -s /usr/local/go121/go/bin/go /usr/local/bin/go_121

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
## IMAGE(build-*): Build individual components

## k8sd build
FROM builder-dqlite AS build-k8sd
WORKDIR /src/k8s-snap/src/k8s
RUN make static -j
RUN mkdir -p /out/bin && mv ./bin/static/* /out/bin/

## k8s-dqlite build
FROM builder-dqlite AS build-k8s-dqlite
RUN /src/k8s-snap/build-scripts/build-component.sh k8s-dqlite

## cni build
FROM builder AS build-cni
RUN /src/k8s-snap/build-scripts/build-component.sh cni

## containerd build
FROM builder AS build-containerd
RUN /src/k8s-snap/build-scripts/build-component.sh containerd

## helm build
FROM builder AS build-helm
RUN /src/k8s-snap/build-scripts/build-component.sh helm

## kubernetes build
FROM builder AS build-kubernetes
RUN /src/k8s-snap/build-scripts/build-component.sh kubernetes

## runc build
FROM builder AS build-runc
RUN /src/k8s-snap/build-scripts/build-component.sh runc

## pebble build
FROM builder AS build-pebble
RUN /src/k8s-snap/build-scripts/build-component.sh pebble

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
COPY --from=build-k8s-dqlite /out /snap/k8s/current
COPY --from=build-runc /out /snap/k8s/current
COPY --from=build-helm /out /snap/k8s/current
COPY --from=build-containerd /out /snap/k8s/current
COPY --from=build-cni /out /snap/k8s/current
COPY --from=build-kubernetes /out /snap/k8s/current
COPY --from=build-k8sd /out /snap/k8s/current
COPY --from=build-pebble /out /snap/k8s/current

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
