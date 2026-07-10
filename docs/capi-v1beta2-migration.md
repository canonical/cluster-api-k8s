                 # Migration: cluster-api v1.9.6 → v1.13.2 (v1beta1 → v1beta2)

## Overview

~40 source files need changes across 6 concern areas (including the `e2e`-tagged `test/e2e` package, which is not covered by `go build ./...`). All changes maintain backward-compatible behavior by using the v1beta2 deprecated compat package for the old-style conditions system.

---

## Area 1 — `go.mod` updates

Direct dependency bumps required:

| Dependency | From | To |
|---|---|---|
| `sigs.k8s.io/cluster-api` | v1.9.6 | v1.13.2 |
| `sigs.k8s.io/cluster-api/test` | v1.9.6 | v1.13.2 |
| `sigs.k8s.io/controller-runtime` | v0.19.6 | v0.23.3 |
| `k8s.io/api` | v0.31.3 | v0.35.4 |
| `k8s.io/apimachinery` | v0.31.3 | v0.35.4 |
| `k8s.io/apiserver` | v0.31.3 | v0.35.4 |
| `k8s.io/client-go` | v0.31.3 | v0.35.4 |
| `k8s.io/kubernetes` | v1.31.3 | v1.35.4 |
| `go` directive | 1.23.0 | 1.25.0 |
| `toolchain` | go1.23.8 | go1.26.1 |

After all source code changes are applied, run `go mod tidy` to update all indirect dependencies.

---

## Area 2 — Import path changes (all ~35 files)

The standard alias for `sigs.k8s.io/cluster-api/api/core/v1beta2` is `clusterv1` everywhere. The new package's Go package name is `v1beta2`, so any file that previously used a bare import (no alias) must add the explicit `clusterv1` alias — otherwise existing code referencing `v1beta1.XXX` would break.

| Old import | New import |
|---|---|
| `clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"` | `clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"` |
| `clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"` | `clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"` + rename all usages `clusterv1beta1.` → `clusterv1.` |
| `"sigs.k8s.io/cluster-api/api/v1beta1"` (no alias, package used as `v1beta1.`) | `clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"` + rename all usages `v1beta1.` → `clusterv1.` |
| `expv1beta1/expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"` | **REMOVE** — MachinePool merged into core v1beta2 |
| `"sigs.k8s.io/cluster-api/util/conditions"` | `conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"` |

MachinePool is now `clusterv1.MachinePool` in the core package; `expv1.MachinePool` usages in test code become `clusterv1.MachinePool`.

---

## Area 3 — API type method renames

Files: `bootstrap/api/v1beta2/ck8sconfig_types.go`, `controlplane/api/v1beta2/ck8scontrolplane_types.go`

The deprecated compat conditions package requires the `GetV1Beta1Conditions`/`SetV1Beta1Conditions` interface instead of the old `GetConditions`/`SetConditions`:

```go
// Before (v1beta1)
func (c *CK8sConfig) GetConditions() clusterv1.Conditions { ... }
func (c *CK8sConfig) SetConditions(conditions clusterv1.Conditions) { ... }

// After (v1beta2)
func (c *CK8sConfig) GetV1Beta1Conditions() clusterv1.Conditions { ... }
func (c *CK8sConfig) SetV1Beta1Conditions(conditions clusterv1.Conditions) { ... }
```

---

## Area 4 — Constant renames

The upstream `sigs.k8s.io/cluster-api/api/core/v1beta2` package itself renamed these constants by adding a `V1Beta1` suffix. This was done to disambiguate them from the new v1beta2-style conditions which use standard `metav1.Condition` and have their own set of constants without the suffix. The old names (e.g. `ControlPlaneInitializedCondition`) simply no longer exist in the package, so our code must use the new names or it will not compile.

In v1beta2 the old v1beta1-style condition/reason constants get a `V1Beta1` suffix:

| Old | New |
|---|---|
| `clusterv1.ControlPlaneInitializedCondition` | `clusterv1.ControlPlaneInitializedV1Beta1Condition` |
| `clusterv1.WaitingForControlPlaneAvailableReason` | `clusterv1.WaitingForControlPlaneAvailableV1Beta1Reason` |
| `clusterv1.ReadyCondition` | `clusterv1.ReadyV1Beta1Condition` |
| `clusterv1.DeletingReason` | `clusterv1.DeletingV1Beta1Reason` |
| `clusterv1.MachineHealthCheckSucceededCondition` | `clusterv1.MachineHealthCheckSucceededV1Beta1Condition` |
| `clusterv1.MachineOwnerRemediatedCondition` | `clusterv1.MachineOwnerRemediatedV1Beta1Condition` |
| `clusterv1.WaitingForRemediationReason` | `clusterv1.WaitingForRemediationV1Beta1Reason` |
| `clusterv1.RemediationFailedReason` | `clusterv1.RemediationFailedV1Beta1Reason` |
| `clusterv1.RemediationInProgressReason` | `clusterv1.RemediationInProgressV1Beta1Reason` |
| `clusterv1.PreTerminateDeleteHookSucceededCondition` | `clusterv1.PreTerminateDeleteHookSucceededV1Beta1Condition` |

---

## Area 5 — Breaking API shape changes

### 5a. `patch.WithOwnedConditions` → `patch.WithOwnedV1Beta1Conditions`

The `WithOwnedConditions` struct now owns new-style `[]metav1.Condition`. Use `WithOwnedV1Beta1Conditions` for the old `[]clusterv1.ConditionType`.

```go
// Before
patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{...}}

// After
patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{...}}
```

### 5b. `collections.OwnedMachines` — new required argument

The filter now requires the owner's `GroupKind` as a second argument.

```go
// Before
collections.OwnedMachines(kcp)

// After
collections.OwnedMachines(kcp, controlplanev1.GroupVersion.WithKind("CK8sControlPlane").GroupKind())
```

Call sites: `ck8scontrolplane_controller.go` (×3), `scale.go` (×1).

### 5c. `cluster.Status.InfrastructureReady` removed

The field moved into the new initialization status sub-struct.

```go
// Before
if !cluster.Status.InfrastructureReady {

// After
if cluster.Status.Initialization.InfrastructureProvisioned == nil || !*cluster.Status.Initialization.InfrastructureProvisioned {
```

Call sites: `bootstrap/controllers/ck8sconfig_controller.go:178`, `controlplane/controllers/ck8scontrolplane_controller.go:107`.

### 5d. Predicate rename

```go
// Before
predicates.ClusterPausedTransitionsOrInfrastructureReady(mgr.GetScheme(), r.Log)

// After
predicates.ClusterPausedTransitionsOrInfrastructureProvisioned(mgr.GetScheme(), r.Log)
```

Call site: `controlplane/controllers/ck8scontrolplane_controller.go:280`.

### 5e. `util.IsOwnedByObject` and `util.IsControlledBy` — new required argument

Both functions now require a `schema.GroupKind` as a third argument.

```go
// Before
util.IsOwnedByObject(&_ms, md)
util.IsControlledBy(configSecret, kcp)

// After
util.IsOwnedByObject(&_ms, md, clusterv1.GroupVersion.WithKind("MachineDeployment").GroupKind())
util.IsControlledBy(configSecret, kcp, controlplanev1.GroupVersion.WithKind("CK8sControlPlane").GroupKind())
```

Call sites: `bootstrap/controllers/orchestrated_inplace_upgrade_controller.go:267`, `controlplane/controllers/ck8scontrolplane_controller.go:618`.

### 5f. `machine.Status.NodeRef` nil-checks

`NodeRef` changed from `*corev1.ObjectReference` to a value-type `MachineNodeReference`. All nil-checks must use `IsDefined()`.

```go
// Before
if machine.Status.NodeRef == nil { ... }
if machine.Status.NodeRef != nil { ... }

// After
if !machine.Status.NodeRef.IsDefined() { ... }
if machine.Status.NodeRef.IsDefined() { ... }
```

`machine.Status.NodeRef.Name` still works unchanged.

Call sites: `workload_cluster.go` (×5), `certificates_controller.go` (×1), `scale.go` (×1), `remediation.go` (×1), `test/e2e/helpers.go` (×2).

### 5g. `FailureDomains` type change — map to slice

`clusterv1.FailureDomains` was `map[string]FailureDomainSpec`. In v1beta2 it is `[]FailureDomain`. All helper methods on the old map type are gone.

```go
// Before — FailureDomains helper methods existed on the map type
c.Cluster.Status.FailureDomains.FilterControlPlane()      // FailureDomains
c.Cluster.Status.FailureDomains.FilterControlPlane().GetIDs()  // []*string
failuredomains.PickMost(ctx, fds, ...)                    // returns *string
collections.InFailureDomains(ids...)                      // takes ...*string

// After — plain slice, no helper methods
// FilterControlPlane: inline the filter
var cpFDs []clusterv1.FailureDomain
for _, fd := range c.Cluster.Status.FailureDomains {
    if fd.ControlPlane != nil && *fd.ControlPlane {
        cpFDs = append(cpFDs, fd)
    }
}
// GetIDs: extract names
names := make([]string, len(cpFDs))
for i, fd := range cpFDs { names[i] = fd.Name }
collections.InFailureDomains(names...)                    // takes ...string now
failuredomains.PickMost(ctx, cpFDs, ...)                  // returns string now (not *string)
```

`Machine.Spec.FailureDomain` also changed from `*string` to `string` — no pointer dereference needed when reading it.

`FailureDomainSpec.ControlPlane bool` → `FailureDomain.ControlPlane *bool` — nil-check required before use.

Call sites: `pkg/ck8s/control_plane.go` (×5), `test/e2e/helpers.go` (FailureDomains map range loop).

---

## Area 6 — test/e2e changes

> **Note:** the `test/e2e` package is gated behind the `e2e` build tag, so a plain
> `go build ./...` does **not** catch breakage here. Verify with
> `go vet -tags e2e ./test/e2e/...` (or `go test -tags e2e -c ./test/e2e/`).

### 6a. Import / scheme changes

- Remove `dockerinfrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"` — the CAPD package was removed from the test module in v1.13.2.
- Remove `Expect(dockerinfrav1.AddToScheme(sc)).To(Succeed())` from `e2e_suite_test.go`.
- Change `expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"` to use `clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"` and update `expv1.MachinePool` → `clusterv1.MachinePool`.

### 6b. `E2EConfig.GetVariable` removed → `MustGetVariable`

The test framework renamed the accessor. The old name no longer exists, so all call sites must be updated (it also added `GetVariableOrEmpty` for the non-fatal case).

```go
// Before
e2eConfig.GetVariable(KubernetesVersion)

// After
e2eConfig.MustGetVariable(KubernetesVersion)
```

23 call sites across: `cluster_upgrade.go`, `create_test.go`, `node_scale_test.go` (×5), `in_place_upgrade_test.go` (×3), `orchestrated_cp_in_place_upgrade_test.go` (×2), `orchestrated_md_in_place_upgrade_test.go` (×2), `md_remediation_test.go` (×2), `refresh_certs_test.go`, `intermediate_ca_test.go`, `e2e_suite_test.go` (×2). (The `e2eConfig.Variables` map field is unchanged.)

### 6c. `DeleteAllClustersAndWaitInput` shape change

The `Client` field was replaced by `ClusterProxy`, and `ClusterctlConfigPath` is now required.

```go
// Before
framework.DeleteAllClustersAndWait(ctx, framework.DeleteAllClustersAndWaitInput{
    Client:    input.ClusterProxy.GetClient(),
    Namespace: input.Namespace.Name,
}, ...)

// After
framework.DeleteAllClustersAndWait(ctx, framework.DeleteAllClustersAndWaitInput{
    ClusterProxy:         input.ClusterProxy,
    ClusterctlConfigPath: clusterctlConfigPath, // package-level global from e2e_suite_test.go
    Namespace:            input.Namespace.Name,
    ArtifactFolder:       input.ArtifactFolder,
}, ...)
```

Call site: `common.go` (`dumpSpecResourcesAndCleanup`).

### 6d. `Cluster.Spec.Topology` value type + `Class` → `ClassRef.Name`

`Spec.Topology` changed from `*Topology` to a value type (use `IsDefined()`), and the `Class string` field became `ClassRef ClusterClassRef`.

```go
// Before
if result.Cluster.Spec.Topology != nil {
    ... Name: result.Cluster.Spec.Topology.Class
}

// After
if result.Cluster.Spec.Topology.IsDefined() {
    ... Name: result.Cluster.Spec.Topology.ClassRef.Name
}
```

Call site: `helpers.go` (`ApplyClusterTemplateAndWait`).

### 6e. `Machine.Spec.Version` `*string` → `string`

Same change as the rest of the migration's pointer→value field moves; drop the deref / address-of.

```go
// Before
if *m.Spec.Version == input.KubernetesUpgradeVersion { ... }
deployment.Spec.Template.Spec.Version = &input.UpgradeVersion
klog ..., *oldVersion, ...

// After
if m.Spec.Version == input.KubernetesUpgradeVersion { ... }
deployment.Spec.Template.Spec.Version = input.UpgradeVersion
klog ..., oldVersion, ...
```

Call sites: `helpers.go` (×3, in the control-plane and MachineDeployment upgrade helpers).

### 6f. `MachineSpec.InfrastructureRef` → `ContractVersionedObjectReference`

The infra ref is no longer a `corev1.ObjectReference`. The new
`ContractVersionedObjectReference` has only `Kind`, `Name`, and `APIGroup` —
**no `APIVersion` and no `Namespace`** (the referenced object is assumed to live
in the same namespace, and the version is resolved from the CRD contract labels).
Instead of constructing a fresh ref, mutate the existing one's `Name` (mirrors
upstream `framework.UpgradeMachineDeploymentsAndWait`).

```go
// Before
newInfrastructureRef := corev1.ObjectReference{
    APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
    Kind:       "DockerMachineTemplate",
    Name:       fmt.Sprintf("%s-md-new-0", input.Cluster.Name),
    Namespace:  deployment.Spec.Template.Spec.InfrastructureRef.Namespace,
}
deployment.Spec.Template.Spec.InfrastructureRef = newInfrastructureRef

// After
deployment.Spec.Template.Spec.InfrastructureRef.Name = fmt.Sprintf("%s-md-new-0", input.Cluster.Name)
```

Call site: `helpers.go` (`UpgradeMachineDeploymentsAndWait`).

### 6g. e2e cluster templates → v1beta2 (`test/e2e/data/infrastructure-docker/*.yaml`)

The 5 Docker cluster templates applied by the specs were still on v1beta1 for the
core (`cluster.x-k8s.io`) and Docker infra (`infrastructure.cluster.x-k8s.io`)
objects. v1beta1 is still *served* in v1.13.2 (so the suite ran with deprecation
warnings), but the templates were migrated to v1beta2 to drop the deprecated APIs.
Three distinct changes were required — a plain version-string bump is **not**
sufficient:

**1. Object `apiVersion` bump** — `Cluster`, `MachineDeployment`, `MachineHealthCheck`
(`cluster.x-k8s.io/v1beta1` → `…/v1beta2`) and `DockerCluster` / `DockerMachineTemplate`
(`infrastructure.cluster.x-k8s.io/v1beta1` → `…/v1beta2`; v1beta2 is the storage version).

**2. Core references change shape: `apiVersion` → `apiGroup`.** In v1beta2,
`Cluster.spec.controlPlaneRef` / `infrastructureRef` and
`MachineDeployment.spec.template.spec.bootstrap.configRef` / `infrastructureRef`
are `ContractVersionedObjectReference` (`apiGroup` + `kind` + `name`; no version, no namespace):

```yaml
# Before
controlPlaneRef:
  apiVersion: controlplane.cluster.x-k8s.io/v1beta2
  kind: CK8sControlPlane
  name: ...
# After
controlPlaneRef:
  apiGroup: controlplane.cluster.x-k8s.io
  kind: CK8sControlPlane
  name: ...
```

> **Do NOT convert** `CK8sControlPlane.spec.machineTemplate.infrastructureTemplate`
> — that is a CK8s-owned field, still a `corev1.ObjectReference`, so it keeps the
> `apiVersion: …/v1beta2` form. Same for every object's own top-level `apiVersion`.

**3. `MachineHealthCheck` spec restructure** (kcp-remediation, md-remediation templates):

| v1beta1 | v1beta2 |
|---|---|
| `spec.maxUnhealthy: 100%` | `spec.remediation.triggerIf.unhealthyLessThanOrEqualTo: 100%` |
| `spec.nodeStartupTimeout: 30s` | `spec.checks.nodeStartupTimeoutSeconds: 30` |
| `spec.unhealthyConditions[]` | `spec.checks.unhealthyNodeConditions[]` |
| `…[].timeout: 10s` | `…[].timeoutSeconds: 10` |

Also a hardcoded ref in **`helpers.go`** (`UpgradeControlPlaneAndWaitForUpgrade`,
the `[CK8s-Upgrade]` spec) bumped: `infrastructure.cluster.x-k8s.io/v1beta1` →
`/v1beta2` (this one is a `corev1.ObjectReference` on the CK8s control plane, so it
keeps `APIVersion`).

**4. `CK8sControlPlane.spec.machineTemplate.metadata` is now required to be non-empty.**
The regenerated CRD (controller-gen v0.21.0) sets `minProperties: 1` on that field
(it became `clusterv1.ObjectMeta`), and an empty `metadata: {}` is serialized on
apply — so a template without it is rejected (`spec.machineTemplate.metadata in body
should have at least 1 properties`). Each template's `machineTemplate` now carries a
minimal metadata block:

```yaml
machineTemplate:
  metadata:
    labels:
      cluster.x-k8s.io/control-plane: ""
  infrastructureTemplate:
    ...
```

Validated: with these changes the templates apply cleanly against a live v1.13.2
management cluster — `Cluster`, `MachineDeployment`, `DockerCluster`,
`DockerMachineTemplate`, `CK8sConfigTemplate`, and `CK8sControlPlane` are all accepted,
and the CK8sControlPlane reconciles and creates machines.

> **Runtime prerequisite (unrelated to this migration):** the templates reference
> node images `customImage: k8s-snap:dev-old` / `dev-new`. These are CK8s node images
> that must be built and loaded locally (see `docs/development.md`); they are **not**
> produced by `make docker-build-e2e`. Without them, workload Machines fail with
> `pull access denied for k8s-snap` and specs time out waiting for nodes — independent
> of the API-version changes above.

> **Not done:** `test/e2e/data/infrastructure-aws/cluster-template.yaml` still uses
> `cluster.x-k8s.io/v1beta1` and `addons.cluster.x-k8s.io/v1beta1`. The AWS path uses
> CAPA's independently-versioned infra types and is not run by default, so it was left
> for a separate, CAPA-version-aware change.

### 6h. Docker infra CRDs → Dev infra CRDs (`Docker*` → `Dev*`)

In v1.13.2 the test infrastructure provider was reworked: the per-backend
`DockerCluster` / `DockerMachine` / `DockerMachineTemplate` kinds are **deprecated** in
favor of unified `Dev*` kinds with a pluggable `backend` (`docker` or `inMemory`).
Same API group/version (`infrastructure.cluster.x-k8s.io/v1beta2`), and the `--infrastructure docker`
provider still ships them — only the kind and spec shape change. The e2e Docker
templates were migrated to drop the deprecated kinds:

| Before | After |
|---|---|
| `kind: DockerCluster` … `spec: {}` | `kind: DevCluster` … `spec.backend.docker: {}` |
| `kind: DockerMachineTemplate` … `spec.template.spec.customImage: <img>` | `kind: DevMachineTemplate` … `spec.template.spec.backend.docker.customImage: <img>` |

```yaml
# DevCluster
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: DevCluster
spec:
  backend:
    docker: {}
---
# DevMachineTemplate
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: DevMachineTemplate
spec:
  template:
    spec:
      backend:
        docker:
          customImage: k8s-snap:dev-old
```

All `infrastructureRef` / `infrastructureTemplate` kinds that pointed at the Docker
kinds were updated to the Dev kinds (the `apiGroup` is unchanged). One Go reference was
also updated: `helpers.go` `UpgradeControlPlaneAndWaitForUpgrade` builds an infra ref
with `Kind: "DockerMachineTemplate"` → `"DevMachineTemplate"`.

10 `DockerCluster` + 24 `DockerMachineTemplate` kind references across the 5 templates,
plus the one Go reference. Verified with `go vet -tags e2e ./test/e2e/...`; full
validation still requires an actual run (see the node-image / arm64 notes in
`docs/e2e-node-images.md`).

---

## Implementation Order

1. **`go.mod`** — update versions
2. **API types** — rename `GetConditions`/`SetConditions` methods (`ck8sconfig_types.go`, `ck8scontrolplane_types.go`)
3. **Generated deepcopy** — fix import paths only (`zz_generated.deepcopy.go` ×2)
4. **Condition consts** — fix import paths + constant renames (`condition_consts.go` ×2)
5. **`main.go` ×2** — remove `expv1beta1` import, rename `clusterv1beta1` alias
6. **Controllers** (6 files) — import paths + conditions pkg + constant renames + API shape changes (5a–5f)
7. **`pkg/`** (12 files) — import paths + NodeRef nil-check changes + conditions pkg + FailureDomains type change in `control_plane.go` (5g)
8. **`test/e2e`** (5 go files) — remove docker infra import, fix MachinePool import, FailureDomains slice iteration, `GetVariable`→`MustGetVariable`, `DeleteAllClustersAndWaitInput` shape, `Topology`/`Version`/`InfrastructureRef` shape changes (6a–6f)
9. **`test/e2e/data/infrastructure-docker`** (5 templates) — object apiVersion bump, core refs `apiVersion`→`apiGroup`, MHC spec restructure (6g); `Docker*`→`Dev*` infra kinds + `backend.docker` spec (6h)
10. **`go mod tidy`** — pull in all transitive dependency updates
11. **`go build ./...`** — verify compilation
12. **`go vet -tags e2e ./test/e2e/...`** — verify the e2e package compiles (not covered by step 11 due to the `e2e` build tag)
13. **`make docker-build-e2e && make test-e2e`** — only an actual run validates the cluster templates apply against a live v1.13.2 management cluster

---

## File Inventory

### Controllers
| File | Changes needed |
|---|---|
| `controlplane/controllers/ck8scontrolplane_controller.go` | import, constants, conditions pkg, `WithOwnedV1Beta1Conditions`, `OwnedMachines` (×3), `InfrastructureProvisioned`, predicate rename, `IsControlledBy` GroupKind arg |
| `controlplane/controllers/scale.go` | import, conditions pkg, `OwnedMachines` (×1), NodeRef |
| `controlplane/controllers/remediation.go` | import, constants, conditions pkg, `WithOwnedV1Beta1Conditions`, NodeRef |
| `controlplane/controllers/machine_controller.go` | import, constant rename |
| `controlplane/controllers/orchestrated_inplace_upgrade_controller.go` | import, `IsOwnedByObject` GroupKind arg |
| `bootstrap/controllers/ck8sconfig_controller.go` | import, constants, conditions pkg, `InfrastructureProvisioned` |
| `bootstrap/controllers/certificates_controller.go` | import, NodeRef |
| `bootstrap/controllers/upgrade_controller.go` | import |
| `bootstrap/controllers/orchestrated_inplace_upgrade_controller.go` | import |

### pkg/
| File | Changes needed |
|---|---|
| `pkg/ck8s/workload_cluster.go` | import, conditions pkg, NodeRef (×5) |
| `pkg/ck8s/control_plane.go` | import, FailureDomains type change (×5), `FilterControlPlane`/`GetIDs` removal, `PickMost`/`PickFewest` signatures, `InFailureDomains` string→string, `Spec.FailureDomain` `string` |
| `pkg/ck8s/management_cluster.go` | import |
| `pkg/ck8s/config_init.go` | import |
| `pkg/machinefilters/machine_filters.go` | import |
| `pkg/machinefilters/machine_filters_test.go` | import |
| `pkg/locking/control_plane_init_mutex.go` | import |
| `pkg/secret/certificates.go` | import |
| `pkg/token/token.go` | import |
| `pkg/token/token_test.go` | import |
| `pkg/upgrade/inplace/inplace.go` | import |
| `pkg/upgrade/inplace/inplace_test.go` | import |
| `pkg/upgrade/inplace/interface.go` | import |
| `pkg/upgrade/inplace/lock.go` | import |
| `pkg/upgrade/inplace/lock_test.go` | import |
| `pkg/upgrade/inplace/mark.go` | import |
| `pkg/upgrade/inplace/mark_test.go` | import |

### test/e2e
| File | Changes needed |
|---|---|
| `test/e2e/e2e_suite_test.go` | remove `dockerinfrav1` import + `AddToScheme` call, `GetVariable`→`MustGetVariable` (×2) (6a, 6b) |
| `test/e2e/helpers.go` | `expv1` → `clusterv1`, NodeRef nil-checks (×2), FailureDomains slice iteration, `Topology` value type + `ClassRef.Name` (6d), `Spec.Version` `*string`→`string` (×3) (6e), `InfrastructureRef` → `ContractVersionedObjectReference` (6f), hardcoded infra ref apiVersion `/v1beta1`→`/v1beta2` (6g), infra ref `Kind` `DockerMachineTemplate`→`DevMachineTemplate` (6h) |
| `test/e2e/common.go` | import path updates, `DeleteAllClustersAndWaitInput` `Client`→`ClusterProxy` + `ClusterctlConfigPath` (6c) |
| `test/e2e/cluster_upgrade.go` | `GetVariable`→`MustGetVariable` (×4) (6b) |
| `test/e2e/refresh_certs_test.go` | import path updates, `GetVariable`→`MustGetVariable` (6b) |
| `test/e2e/create_test.go`, `node_scale_test.go`, `in_place_upgrade_test.go`, `orchestrated_cp_in_place_upgrade_test.go`, `orchestrated_md_in_place_upgrade_test.go`, `md_remediation_test.go`, `intermediate_ca_test.go` | `GetVariable`→`MustGetVariable` (6b) |
| `test/e2e/data/infrastructure-docker/*.yaml` (5 templates) | object apiVersion `v1beta1`→`v1beta2`, core refs `apiVersion`→`apiGroup`, MHC spec restructure to `checks`/`remediation` (6g); `Docker*`→`Dev*` infra kinds with `backend.docker` spec (6h) |
