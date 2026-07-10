# E2E node images (k8s-snap) and how to build them

The Docker-based e2e suite provisions workload clusters whose nodes run
[Canonical Kubernetes (`k8s-snap`)](https://github.com/canonical/k8s-snap). Those
nodes are CAPD containers started from a **custom node image that has `k8s-snap`
baked in**. This image is a separate build artifact from the provider controllers,
and the suite will not pass without it.

This doc explains the dependency, the images involved, and how to build them with
`hack/build-e2e-images.sh`.

## Why this is needed

`make docker-build-e2e` builds only the **controller** images
(`bootstrap-controller:dev`, `controlplane-controller:dev`). It does **not** build
the node images.

The e2e cluster templates (`test/e2e/data/infrastructure-docker/*.yaml`) reference
the node images directly on each `DockerMachineTemplate`:

```yaml
kind: DockerMachineTemplate
spec:
  template:
    spec:
      customImage: k8s-snap:dev-old   # (or k8s-snap:dev-new for the post-upgrade template)
```

If these images are not present in the local Docker daemon, CAPD cannot create the
machine containers and the specs fail / time out:

```
failed to create DockerMachine: error pulling container image k8s-snap:dev-old:
pull access denied for k8s-snap, repository does not exist or may require 'docker login'
```

(These are local-only tags — there is no registry to pull them from, so they must be
built locally.)

## The two images

| Tag | Built with | Used by |
|---|---|---|
| `k8s-snap:dev-old` | `KUBERNETES_VERSION=v1.32.1`, `KUBERNETES_VERSION_UPGRADE_TO=v1.33.0` | The initial node image for every cluster; also the source image for **in-place** upgrade specs (it stages the upgrade-to version inside the same image) |
| `k8s-snap:dev-new` | `KUBERNETES_VERSION=v1.33.0` | The post-upgrade node image for the **rolling** upgrade specs (`[CK8s-Upgrade]`), referenced by the `*-new`/`*-md-new-*` `DockerMachineTemplate`s |

These versions match the e2e config (`test/e2e/config/ck8s-docker.yaml`):

```yaml
KUBERNETES_VERSION:            "v1.32.1"   # initial workload version
KUBERNETES_VERSION_UPGRADE_TO: "v1.33.0"   # upgrade target
```

> Keep the build args, the e2e config versions, and the `DockerMachineTemplate`
> `customImage` tags in sync. If you change `KUBERNETES_VERSION` /
> `KUBERNETES_VERSION_UPGRADE_TO`, rebuild the images with matching args.

## Build the images: `hack/build-e2e-images.sh`

This script is the canonical way to produce both node images. It runs two
`docker build`s against `templates/docker/Dockerfile`:

```bash
./hack/build-e2e-images.sh
```

which is equivalent to:

```bash
cd templates/docker
docker build . --platform linux/amd64 -t k8s-snap:dev-old \
  --build-arg BRANCH=release-1.32 \
  --build-arg KUBERNETES_VERSION=v1.32.1 \
  --build-arg KUBERNETES_VERSION_UPGRADE_TO=v1.33.0
docker build . --platform linux/amd64 -t k8s-snap:dev-new \
  --build-arg BRANCH=release-1.33 \
  --build-arg KUBERNETES_VERSION=v1.33.0
```

The script builds **`linux/amd64`** by default because the k8s-snap / dqlite C build
does not compile natively on arm64 (see the limitation below). It uses the
version-matched k8s-snap **release branches** rather than the moving `main` branch
(which drifts from the Dockerfile's expected source layout).

Both knobs are overridable via environment variables:

```bash
# native build on an amd64 Linux host (no emulation)
PLATFORM=linux/amd64 ./hack/build-e2e-images.sh

# rootful docker daemon
DOCKER="sudo docker" ./hack/build-e2e-images.sh
```

### What the build does

`templates/docker/Dockerfile` clones `canonical/k8s-snap` (at `BRANCH`), builds the
`k8s-snap` components and `k8sd`, overrides the Kubernetes version with the
`KUBERNETES_VERSION` build arg (and additionally builds `KUBERNETES_VERSION_UPGRADE_TO`
when set, so a single image can serve an in-place upgrade), and assembles everything
on top of a `kindest/node` base so the result is usable as a CAPD node.

### Build args reference

| Arg | Default | Meaning |
|---|---|---|
| `REPO` | `https://github.com/canonical/k8s-snap` | k8s-snap source repository |
| `BRANCH` | `main` | branch/tag of k8s-snap to build |
| `KUBERNETES_VERSION` | _(empty)_ | overrides the Kubernetes version baked into the snap |
| `KUBERNETES_VERSION_UPGRADE_TO` | _(empty)_ | additionally builds this version (for in-place upgrade tests) |
| `BASE` | `kindest/node:v1.32.2` | node base image (keep aligned with the k8s-snap minor version) |

### Script environment variables (`hack/build-e2e-images.sh`)

| Var | Default | Meaning |
|---|---|---|
| `DOCKER` | `docker` | docker command to use; the local daemon (rootless / Colima friendly). Use `DOCKER="sudo docker"` for a rootful daemon |
| `PLATFORM` | `linux/amd64` | target build platform. amd64 by default because the dqlite C build does not compile natively on arm64 (see below) |

> **Build cost:** these images compile k8s-snap from source — expect a long first
> build. They are cached afterward, so you only rebuild when the branch or versions
> change. Building `linux/amd64` on an arm64 host runs under QEMU emulation, which is
> slower still.

## Full Docker e2e run sequence

```bash
# 1. Build the provider controller images (tag: dev)
make docker-build-e2e

# 2. Build the k8s-snap node images (k8s-snap:dev-old, k8s-snap:dev-new)
./hack/build-e2e-images.sh

# 3. Verify both node images exist in the local daemon
docker images | grep k8s-snap
#   k8s-snap   dev-old   ...
#   k8s-snap   dev-new   ...

# 4. Run the suite (or a focused subset)
make test-e2e
make GINKGO_FOCUS="\\[PR-Blocking\\]" test-e2e
```

Note: the node images do **not** need to be `kind load`-ed into the management
cluster. CAPD starts workload "node" containers directly from the host Docker
daemon, so they only need to exist locally (step 3).

## Known limitation: Apple Silicon / arm64 hosts

These images **cannot currently be built natively on an `arm64` host** (Apple
Silicon + Colima/Docker Desktop). The k8s-snap build compiles dqlite / k8s-dqlite
from C + cgo, and that chain has **multiple, version-dependent arm64 build failures**:

- **k8s-dqlite v1.3.5** (k8s-snap `release-1.32` / `release-1.33`): the static musl
  build passes the x86_64-only flag `-m64`, rejected by the arm64 toolchain:
  ```
  cc1: error: unrecognized command line option '-m64'
  make: *** [bin/static/k8s-dqlite] Error 1
  ```
- **k8s-dqlite v1.8.1** (k8s-snap `release-1.34` / `release-1.35`): `-m64` is fixed
  and the build gets much further, but the dqlite C library then fails on an
  arm64 narrowing-conversion warning treated as an error:
  ```
  src/vfs.c: error: conversion from 'int' to 'uint16_t' may change value [-Werror=conversion]
  make: *** [bin/dynamic/lib/libdqlite.so] Error 2
  ```

These originate in upstream k8s-snap / dqlite, not in this repo, and each is just the
next blocker in the chain — not worth patching transitively-cloned C source.

To work around this, `hack/build-e2e-images.sh` defaults to **`PLATFORM=linux/amd64`**,
so the C flags are valid and the images compile regardless of host architecture.
There are two consequences on an Apple Silicon / arm64 host:

- **Build:** runs under QEMU emulation — it works, but it is slow.
- **Run:** the resulting amd64 node containers also run under emulation when CAPD
  starts them. A full Kubernetes control plane under user-mode emulation is slow and
  unstable (health-check timeouts, crashes), so the e2e suite is **not reliable** this
  way. The amd64 build fixes compilation, not execution.

**Recommendation: build and run the Docker e2e suite on an `amd64` Linux host or in
CI** (this is how upstream produces these images). There, `PLATFORM=linux/amd64` is a
native build and the nodes run natively.

## Troubleshooting

- **`pull access denied for k8s-snap`** — the node images aren't in the daemon CAPD
  uses. Run `hack/build-e2e-images.sh` (and drop `sudo` if you use rootless/Colima),
  then confirm with `docker images | grep k8s-snap`.
- **Specs time out at "Waiting for one control plane node to exist"** — same root
  cause: the control-plane container never starts because its `customImage` is
  missing. Build the node images first.
- **Wrong Kubernetes version after a config change** — rebuild the images so the
  `--build-arg` versions match `KUBERNETES_VERSION` / `KUBERNETES_VERSION_UPGRADE_TO`
  in `test/e2e/config/ck8s-docker.yaml`.

## Related

- Provider/runtime dependency on `k8s-snap`: the bootstrap provider installs the snap
  on real nodes via cloud-init (`pkg/cloudinit/scripts/install.sh`,
  `pkg/cloudinit/common.go`) and builds its config against the
  `github.com/canonical/k8s-snap-api` Go module.
- `docs/development.md` — manual `k8s-snap:dev` image build and dev workflow.
- `test/e2e/README.md` — general e2e instructions.
