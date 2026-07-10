# CAPI Breaking Changes: v1.9.6 → v1.13.2

This document lists all upstream cluster-api breaking changes across the v1.10, v1.11, v1.12, and v1.13 releases, notes which ones affect this repository (`cluster-api-k8s`), and cross-references the migration design doc.

Design doc: [capi-v1beta2-migration.md](./capi-v1beta2-migration.md)  
Official migration guides (in `sigs.k8s.io/cluster-api@v1.13.2/docs/book/src/developer/providers/migrations/`):
- `v1.10-to-v1.11.md`
- `v1.11-to-v1.12.md`
- `v1.12-to-v1.13.md`

---

## Legend

| Symbol | Meaning |
|---|---|
| **[AFFECTS REPO]** | Change affects this repository — code must be updated |
| **[NOT AFFECTED]** | Change does not affect this repository |
| **[DESIGN DOC: Area N]** | Addressed in migration design doc |
| **[DESIGN DOC: MISSING]** | Not yet covered in migration design doc — gap to fill |

---

## v1.10 → v1.11 (Major — API v1beta2 introduction)

This is the largest migration step. The upstream team introduced v1beta2 as the new primary API version and reorganized all package paths.

### Go and dependencies

| Change | Affects repo | Addressed |
|---|---|---|
| Minimum Go version bumped to 1.24.x | **[AFFECTS REPO]** go.mod update | **[DESIGN DOC: Area 1]** |
| controller-runtime bumped to v0.21.x | **[AFFECTS REPO]** go.mod update | **[DESIGN DOC: Area 1]** |
| k8s.io/* libraries bumped to v1.33.x | **[AFFECTS REPO]** go.mod update | **[DESIGN DOC: Area 1]** |

### API package path reorganization

All API packages moved to a top-level `api/` folder.

| Old import path | New import path | Affects repo | Addressed |
|---|---|---|---|
| `sigs.k8s.io/cluster-api/api/v1beta1` | `sigs.k8s.io/cluster-api/api/core/v1beta2` | **[AFFECTS REPO]** all 35 files | **[DESIGN DOC: Area 2]** |
| `sigs.k8s.io/cluster-api/exp/api/v1beta1` | merged into `api/core/v1beta2` — **REMOVE** | **[AFFECTS REPO]** `bootstrap/main.go`, `controlplane/main.go`, `test/e2e/helpers.go` | **[DESIGN DOC: Area 2]** |
| `sigs.k8s.io/cluster-api/api/v1beta1` (kept as `api/core/v1beta1`) | deprecated compat path | **[NOT AFFECTED]** we go straight to v1beta2 | — |
| `sigs.k8s.io/cluster-api/util/conditions` | `sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1` (for v1beta1-style conditions) | **[AFFECTS REPO]** all files using conditions | **[DESIGN DOC: Area 2]** |

Note: `exp/api/v1beta1.MachinePool` is now `api/core/v1beta2.MachinePool`. Removing the `exp` import and using `clusterv1.MachinePool` is sufficient.

### conditions system overhaul

| Change | Affects repo | Addressed |
|---|---|---|
| `status.conditions` on all types changed from `clusterv1.Conditions` to `[]metav1.Condition` | **[AFFECTS REPO]** — keeping old system via deprecated compat package | **[DESIGN DOC: Area 3]** |
| Deprecated compat package `util/conditions/deprecated/v1beta1` introduced for gradual migration | **[AFFECTS REPO]** — we use this to preserve existing behavior | **[DESIGN DOC: Area 2]** |
| `Getter` interface changed: `GetConditions()` → `GetV1Beta1Conditions()` | **[AFFECTS REPO]** `ck8sconfig_types.go`, `ck8scontrolplane_types.go` | **[DESIGN DOC: Area 3]** |
| `Setter` interface changed: `SetConditions()` → `SetV1Beta1Conditions()` | **[AFFECTS REPO]** same files | **[DESIGN DOC: Area 3]** |
| All v1beta1-style condition/reason constants renamed with `V1Beta1` suffix | **[AFFECTS REPO]** ~10 constants across 5 files | **[DESIGN DOC: Area 4]** |
| `patch.WithOwnedConditions` split: `WithOwnedV1Beta1Conditions` (old style) vs `WithOwnedConditions` (new `metav1.Condition` style) | **[AFFECTS REPO]** `ck8scontrolplane_controller.go`, `remediation.go` | **[DESIGN DOC: Area 5a]** |

### ClusterStatus breaking changes

| Change | Affects repo | Addressed |
|---|---|---|
| `status.infrastructureReady bool` removed — moved to `status.initialization.infrastructureProvisioned *bool` | **[AFFECTS REPO]** 2 call sites | **[DESIGN DOC: Area 5c]** |
| `status.controlPlaneReady bool` removed — replaced by conditions | **[NOT AFFECTED]** not referenced in our code | — |
| `status.failureReason` / `status.failureMessage` removed — moved to `status.deprecated.v1beta1` | **[NOT AFFECTED]** not referenced in our code | — |
| `status.conditions` changed from `Conditions` to `[]metav1.Condition` | **[AFFECTS REPO]** handled via deprecated compat package | **[DESIGN DOC: Area 2, 3]** |
| `status.failureDomains` changed from `map[string]FailureDomainSpec` to `[]FailureDomain` | **[AFFECTS REPO]** `control_plane.go` (×5), `test/e2e/helpers.go` | **[DESIGN DOC: MISSING]** |

### FailureDomains type change (detailed)

This is a multi-part breaking change with cascading effects:

| Sub-change | Affects repo | Addressed |
|---|---|---|
| `clusterv1.FailureDomains` type removed (was `map[string]FailureDomainSpec`) | **[AFFECTS REPO]** `control_plane.go` return type | **[DESIGN DOC: MISSING]** |
| `FailureDomains.FilterControlPlane()` method removed | **[AFFECTS REPO]** `control_plane.go:152,160,165` | **[DESIGN DOC: MISSING]** |
| `FailureDomains.GetIDs()` method removed | **[AFFECTS REPO]** `control_plane.go:152` | **[DESIGN DOC: MISSING]** |
| `FailureDomainSpec.ControlPlane bool` → `FailureDomain.ControlPlane *bool` | **[AFFECTS REPO]** any ControlPlane boolean check on FD | **[DESIGN DOC: MISSING]** |
| `failuredomains.PickMost` signature: `clusterv1.FailureDomains` → `[]clusterv1.FailureDomain` and return `*string` → `string` | **[AFFECTS REPO]** `control_plane.go:160` | **[DESIGN DOC: MISSING]** |
| `failuredomains.PickFewest` signature: same changes as PickMost | **[AFFECTS REPO]** `control_plane.go:168` | **[DESIGN DOC: MISSING]** |
| `collections.InFailureDomains` changed from `...*string` to `...string` | **[AFFECTS REPO]** `control_plane.go:131,152` | **[DESIGN DOC: MISSING]** |
| `Machine.Spec.FailureDomain` changed from `*string` to `string` | **[AFFECTS REPO]** `control_plane.go:158` | **[DESIGN DOC: MISSING]** |
| `test/e2e/helpers.go:463` iterates FailureDomains as a map `range` | **[AFFECTS REPO]** must change to slice iteration | **[DESIGN DOC: MISSING]** |

### MachineStatus breaking changes

| Change | Affects repo | Addressed |
|---|---|---|
| `status.nodeRef` changed from `*corev1.ObjectReference` to `MachineNodeReference` (value type, only has `Name`) | **[AFFECTS REPO]** nil-checks in 8 files | **[DESIGN DOC: Area 5e]** |
| `Machine.Status.NodeRef.Namespace` removed | **[NOT AFFECTED]** our code only uses `.Name` | — |
| `status.bootstrapReady` / `status.infrastructureReady` removed — moved to `status.initialization` | **[NOT AFFECTED]** not referenced in our code | — |
| `status.failureReason` / `status.failureMessage` removed — moved to `status.deprecated.v1beta1` | **[NOT AFFECTED]** not referenced in our code | — |

### MachineSpec breaking changes

| Change | Affects repo | Addressed |
|---|---|---|
| `spec.version` changed from `*string` to `string` | **[AFFECTS REPO]** anywhere `*machine.Spec.Version` is dereferenced | **[DESIGN DOC: MISSING]** |
| `spec.providerID` changed from `*string` to `string` | **[AFFECTS REPO]** anywhere `*machine.Spec.ProviderID` is dereferenced | **[DESIGN DOC: MISSING]** |
| `spec.failureDomain` changed from `*string` to `string` | **[AFFECTS REPO]** `control_plane.go:158` | **[DESIGN DOC: MISSING]** |
| `spec.nodeDeletionTimeout`, `spec.nodeDrainTimeout`, `spec.nodeVolumeDetachTimeout` removed — moved to `spec.deletion.*Seconds` | **[NOT AFFECTED]** not referenced in our code | — |

### util function signature changes

| Change | Affects repo | Addressed |
|---|---|---|
| `util.IsOwnedByObject(obj, target)` → `util.IsOwnedByObject(obj, target, targetGK schema.GroupKind)` | **[AFFECTS REPO]** `orchestrated_inplace_upgrade_controller.go:267` | **[DESIGN DOC: MISSING]** |
| `util.IsControlledBy(obj, owner)` → `util.IsControlledBy(obj, owner, ownerGK schema.GroupKind)` | **[AFFECTS REPO]** `ck8scontrolplane_controller.go:618` | **[DESIGN DOC: MISSING]** |
| `collections.OwnedMachines(owner)` → `collections.OwnedMachines(owner, ownerGK schema.GroupKind)` | **[AFFECTS REPO]** 4 call sites | **[DESIGN DOC: Area 5b]** |

### predicates changes

| Change | Affects repo | Addressed |
|---|---|---|
| `predicates.ClusterPausedTransitionsOrInfrastructureReady` renamed to `ClusterPausedTransitionsOrInfrastructureProvisioned` | **[AFFECTS REPO]** `ck8scontrolplane_controller.go:280` | **[DESIGN DOC: Area 5d]** |

### Scheme registration changes

| Change | Affects repo | Addressed |
|---|---|---|
| `clusterv1.AddToScheme` now includes MachinePool (was in `exp/api/v1beta1.AddToScheme`) | **[AFFECTS REPO]** removes need for separate `expv1beta1.AddToScheme` in `main.go` | **[DESIGN DOC: Area 2]** |
| `clusterv1.GroupVersion` string is now `cluster.x-k8s.io/v1beta2` (was `v1beta1`) | **[AFFECTS REPO]** any code embedding the group version string | **[DESIGN DOC: MISSING]** |

### Removals

| Change | Affects repo | Addressed |
|---|---|---|
| `controllers/remote.ClusterCacheTracker` and related types removed | **[NOT AFFECTED]** we use `remote.RESTConfig` which still exists | — |
| `ClusterStatus` struct in kubeadm bootstrap API group removed | **[NOT AFFECTED]** | — |

### test infrastructure

| Change | Affects repo | Addressed |
|---|---|---|
| `sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1` removed from test module | **[AFFECTS REPO]** `e2e_suite_test.go` | **[DESIGN DOC: Area 6]** |

---

## v1.11 → v1.12

### Go and dependencies

| Change | Affects repo | Addressed |
|---|---|---|
| controller-runtime bumped to v0.22.x | **[AFFECTS REPO]** go.mod update | **[DESIGN DOC: Area 1]** |
| k8s.io/* libraries bumped to v1.34.x | **[AFFECTS REPO]** go.mod update | **[DESIGN DOC: Area 1]** |

### API changes (additive only, no breaking for this repo)

| Change | Affects repo | Addressed |
|---|---|---|
| New `spec.taint` field on Machine | **[NOT AFFECTED]** we don't set it | — |
| New `Updating` Machine condition and phase | **[NOT AFFECTED]** we don't reference these constants | — |
| New `spec.checks.unhealthyMachineConditions` on MachineHealthCheck | **[NOT AFFECTED]** | — |
| `controlplane.cluster.x-k8s.io/kubeadm-cluster-configuration` annotation removed from KCP machines | **[NOT AFFECTED]** | — |
| `util.IsOwnedByObject`, `util.IsControlledBy`, `collections.OwnedMachines` — `schema.GroupKind` parameter added | **[AFFECTS REPO]** (same as v1.11 change above) | **[DESIGN DOC: MISSING / Area 5b]** |

### Suggested provider changes

- `ReconcilerRateLimiting` feature gate introduced (enabled via feature flag, not a code breaking change)
- `PriorityQueue` feature gate introduced

---

## v1.12 → v1.13

### Go and dependencies

| Change | Affects repo | Addressed |
|---|---|---|
| Go minimum version v1.25.0 (used in v1.13.2 `go.mod`) | **[AFFECTS REPO]** go.mod update | **[DESIGN DOC: Area 1]** |
| k8s.io/* libraries bumped to v1.35.x | **[AFFECTS REPO]** go.mod update | **[DESIGN DOC: Area 1]** |

### Removals

| Change | Affects repo | Addressed |
|---|---|---|
| `util/version.ParseMajorMinorPatch` removed | **[NOT AFFECTED]** not used in our code | — |
| `util/version.ParseMajorMinorPatchTolerant` removed | **[NOT AFFECTED]** not used in our code | — |
| `util/topology.ShouldSkipImmutabilityChecks` removed | **[NOT AFFECTED]** not used in our code | — |
| `ClusterCache.GetClientCertificatePrivateKey` removed | **[NOT AFFECTED]** not used in our code | — |
| `--cluster-concurrency` CABPK flag removed | **[NOT AFFECTED]** | — |
| `--disable-grouping` clusterctl flag removed | **[NOT AFFECTED]** | — |
| v1alpha3 + v1alpha4 API versions removed | **[NOT AFFECTED]** we never used them | — |

### API additions (non-breaking)

| Change | Affects repo | Addressed |
|---|---|---|
| New `spec.topology.controlPlane.rollout.taints` field | **[NOT AFFECTED]** | — |
| New `status.failureDomain` field on Machine | **[NOT AFFECTED]** | — |
| New `NodeKubeadmLabelsAndTaintsSet` condition for KCP machines | **[NOT AFFECTED]** | — |
| Runtime hooks now use v1beta2 `Cluster` type | **[NOT AFFECTED]** we don't implement Runtime hooks | — |

### PriorityQueue + ReconcilerRateLimiting

Both feature gates graduated to beta and are **enabled by default** in v1.13. This means all reconcilers are rate-limited to at most 1 request/second by default. This should not require code changes but may affect reconciliation throughput in tests.

---

## Summary: Gaps in the Design Doc

The following `[DESIGN DOC: MISSING]` items need to be added to `capi-v1beta2-migration.md`:

### Gap 1 — `FailureDomains` type change (`pkg/ck8s/control_plane.go`)

`clusterv1.FailureDomains` (old: `map[string]FailureDomainSpec`) becomes `[]clusterv1.FailureDomain`. No helper methods (`FilterControlPlane`, `GetIDs`) exist on the slice.

Required changes in `control_plane.go`:
- `FailureDomains()` method return type: `clusterv1.FailureDomains` → `[]clusterv1.FailureDomain`
- `FilterControlPlane()` calls → inline filter: `fd.ControlPlane != nil && *fd.ControlPlane`
- `GetIDs()` calls → extract `fd.Name` from each element
- `collections.InFailureDomains` now takes `...string` not `...*string`
- `failuredomains.PickMost/PickFewest` return `string` not `*string`
- `Machine.Spec.FailureDomain` is `string` not `*string`

Required changes in `test/e2e/helpers.go:463`:
- `for fd, fdSettings := range cluster.Status.FailureDomains` (map iteration) → `for _, fd := range cluster.Status.FailureDomains` (slice iteration), access `fd.Name` and `fd.ControlPlane`

### Gap 2 — `util.IsOwnedByObject` new signature

```go
// Before
util.IsOwnedByObject(&_ms, md)

// After
util.IsOwnedByObject(&_ms, md, clusterv1.GroupVersion.WithKind("MachineDeployment").GroupKind())
```

Call site: `bootstrap/controllers/orchestrated_inplace_upgrade_controller.go:267`

### Gap 3 — `util.IsControlledBy` new signature

```go
// Before
util.IsControlledBy(configSecret, kcp)

// After
util.IsControlledBy(configSecret, kcp, controlplanev1.GroupVersion.WithKind("CK8sControlPlane").GroupKind())
```

Call site: `controlplane/controllers/ck8scontrolplane_controller.go:618`

### Gap 4 — `Machine.Spec.Version` / `ProviderID` / `FailureDomain` pointer removal

In v1beta2 these are `string` not `*string`. Any code that dereferences `*machine.Spec.Version` or compares `machine.Spec.FailureDomain` as a pointer needs updating.

```go
// Before
notInFailureDomains.Oldest().Spec.FailureDomain  // *string

// After
notInFailureDomains.Oldest().Spec.FailureDomain  // string (no dereference needed)
```

Call sites: `control_plane.go:158`; any code in `workload_cluster.go` or controllers that uses `machine.Spec.Version`.

### Gap 5 — `clusterv1.GroupVersion` string value change

`clusterv1.GroupVersion.String()` now returns `cluster.x-k8s.io/v1beta2` instead of `v1beta1`. Any code that embeds this string in annotations, labels, or log messages will silently change. No code change required but should be verified.

---

## Updated File Inventory (additions to design doc)

| File | Additional changes needed |
|---|---|
| `pkg/ck8s/control_plane.go` | FailureDomains type change, FilterControlPlane/GetIDs removal, PickMost/PickFewest signatures, InFailureDomains `string` not `*string`, `Spec.FailureDomain` is `string` |
| `bootstrap/controllers/orchestrated_inplace_upgrade_controller.go` | `util.IsOwnedByObject` new GroupKind arg |
| `controlplane/controllers/ck8scontrolplane_controller.go` | `util.IsControlledBy` new GroupKind arg (in addition to existing items) |
| `test/e2e/helpers.go` | FailureDomains map iteration → slice iteration (in addition to NodeRef nil-checks) |
