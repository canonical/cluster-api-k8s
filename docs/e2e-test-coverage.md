# E2E Test Coverage

This document describes the end-to-end (e2e) test suite for `cluster-api-k8s`
(the Canonical Kubernetes / CK8s bootstrap and control-plane providers).

## Overview

- **Location:** `test/e2e/`
- **Framework:** [Cluster API test framework](https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework) with [Ginkgo](https://onsi.github.io/ginkgo/) / [Gomega](https://onsi.github.io/gomega/)
- **Infrastructure providers:**
  - **CAPD (Docker)** — default, requires Docker + kind
  - **AWS** — selected via `E2E_INFRA=aws` (requires `AWS_B64ENCODED_CREDENTIALS`)
- **Build tag:** all e2e test files are gated behind the `e2e` build tag.
- **Config:** `test/e2e/config/ck8s-docker.yaml` and `test/e2e/config/ck8s-aws.yaml`
- **Cluster templates:** `test/e2e/data/infrastructure-docker/` and `test/e2e/data/infrastructure-aws/`

### Running the tests

```shell
make docker-build-e2e   # build the controller image (tag: dev) — run after any controller code change
make test-e2e           # run all e2e tests

# Run only PR-blocking tests
make GINKGO_FOCUS="\\[PR-Blocking\\]" test-e2e

# Use an existing (e.g. Tilt-managed) management cluster
make USE_EXISTING_CLUSTER=true test-e2e

# Run on AWS
make E2E_INFRA=aws test-e2e
```

See `test/e2e/README.md` for full setup details (including Tilt-based AWS runs and cleanup guidance).

## Test scenarios

| Test file | Scenario | Topology | Key assertions |
|---|---|---|---|
| `create_test.go` | **`[PR-Blocking]`** Basic cluster creation | 1 CP + 3 workers | Cluster, control plane, and worker MachineDeployments come up healthy |
| `node_scale_test.go` | Scaling up and down | Start 1 CP + 1 worker | Scale workers 1→3, CP 1→6, CP 6→3, workers 3→1; verifies the **k8sd cluster member count** matches at each step |
| `cluster_upgrade_test.go` | **`[CK8s-Upgrade]`** Kubernetes version upgrade | 3 CP (HA) + 1 worker | Two variants — default (`MaxSurge=1`) and **`MaxSurge=0`**; upgrades control plane then MachineDeployments, then verifies all nodes are Ready at the new version |
| `in_place_upgrade_test.go` | **`[PR-Blocking]`** In-place upgrade (per machine) | 1 CP + 1 worker | Applies in-place upgrade to control-plane machines and worker machines separately via a local path |
| `orchestrated_cp_in_place_upgrade_test.go` | **`[CK8SCP-InPlace] [PR-Blocking]`** Orchestrated in-place upgrade | 3 CP + 1 worker | In-place upgrade driven through the **CK8sControlPlane** object (rollout orchestration) |
| `orchestrated_md_in_place_upgrade_test.go` | **`[MD-InPlace] [PR-Blocking]`** Orchestrated in-place upgrade | 1 CP + 3 workers | In-place upgrade driven through the **MachineDeployment** object |
| `refresh_certs_test.go` | **`[PR-Blocking]`** Certificate refresh | 1 CP + 1 worker | Refreshes certs (TTL `1y`) on CP and worker nodes; verifies the `MachineCertificatesExpiryDate` annotation is set, refresh status is `Done`, and the expiry date parses as RFC3339 |
| `intermediate_ca_test.go` | Intermediate CA support | 3 CP + 3 workers | Generates a self-signed root → intermediate CA, seeds the `-ca` / `-cca` / `-proxy` secrets, and verifies the cluster comes up using the externally provided CA certificates |
| `kcp_remediation_test.go` | KCP machine remediation | CAPI shared spec | Uses upstream `KCPRemediationSpec`. **Skipped on AWS.** |
| `md_remediation_test.go` | MachineDeployment remediation | 1 CP + 1 worker | Marks a machine unhealthy via a MachineHealthCheck, waits for remediation, then waits for nodes Ready. **Skipped on AWS.** |

## Coverage themes

- **Lifecycle:** create, scale up/down (control plane and workers), delete (cleanup in `AfterEach` via `dumpSpecResourcesAndCleanup`).
- **Upgrades:**
  - Rolling Kubernetes version upgrade (`MaxSurge=0` and `MaxSurge=1`).
  - Three flavors of **in-place upgrade** (per-machine, orchestrated via CK8sControlPlane, orchestrated via MachineDeployment) — CK8s-specific features.
- **Certificates:** certificate refresh and intermediate-CA provisioning.
- **Resilience:** KCP and MachineDeployment remediation (MachineHealthCheck-driven, Docker only).

## PR-blocking subset

The following are tagged `[PR-Blocking]` and form the gate for pull requests:

- Cluster creation (`create_test.go`)
- In-place upgrade — per machine (`in_place_upgrade_test.go`)
- Orchestrated in-place upgrade — CK8sControlPlane (`orchestrated_cp_in_place_upgrade_test.go`)
- Orchestrated in-place upgrade — MachineDeployment (`orchestrated_md_in_place_upgrade_test.go`)
- Certificate refresh (`refresh_certs_test.go`)

The heavier upgrade, scale, intermediate-CA, and remediation tests run in the fuller suite.

## Known gaps & caveats

- **Remediation tests do not run on AWS** (a known CAPA limitation — see
  [cluster-api-provider-aws#4198](https://github.com/kubernetes-sigs/cluster-api-provider-aws/issues/4198)).
- The **MD remediation test only asserts that the MachineHealthCheck applies the
  unhealthy condition** — it does not yet verify the unhealthy machine is deleted
  and replaced (flagged as a TODO at `md_remediation_test.go:105`).
- No explicit coverage for etcd backup/restore, CNI failure scenarios, or
  multi-cluster topologies.

## Shared helpers

- `helpers.go` — CK8s-specific spec helpers: `ApplyClusterTemplateAndWait`,
  `ApplyInPlaceUpgradeForControlPlane` / `ForWorker` / `ForCK8sControlPlane` / `ForMachineDeployment`,
  `ApplyCertificateRefreshForControlPlane` / `ForWorker`, `WaitForNodesReady`, etc.
- `cluster_upgrade.go` — the reusable `ClusterUpgradeSpec` used by `cluster_upgrade_test.go`.
- `common.go`, `util.go` — namespace setup, cleanup, and k8sd cluster-member helpers.
- `e2e_suite_test.go` — suite bootstrap (management cluster, provider install, config loading).

## CAPI v1.13.2 (v1beta2) migration follow-ups

The CAPI bump to v1.13.2 (`v1beta1` → `v1beta2`) required additional fixes in the
e2e suite beyond the import-path/type changes captured in
[`capi-v1beta2-migration.md`](./capi-v1beta2-migration.md). These were needed for
the `e2e`-tagged package to compile:

| Change | Location | Fix |
|---|---|---|
| `E2EConfig.GetVariable` removed | 23 call sites across all `*_test.go` + `cluster_upgrade.go`, `e2e_suite_test.go` | Renamed to `MustGetVariable` |
| `DeleteAllClustersAndWaitInput.Client` removed; `ClusterctlConfigPath` now required | `common.go` | Use `ClusterProxy` + pass `clusterctlConfigPath` and `ArtifactFolder` |
| `Cluster.Spec.Topology` is now a value type; `Class` (string) → `ClassRef.Name` | `helpers.go` | `Topology.IsDefined()` + `Topology.ClassRef.Name` |
| `Machine.Spec.Version` `*string` → `string` | `helpers.go` (×3) | Drop pointer deref / address-of |
| `MachineSpec.InfrastructureRef` → `ContractVersionedObjectReference` (no `APIVersion`/`Namespace`) | `helpers.go` (`UpgradeMachineDeploymentsAndWait`) | Mutate existing ref's `.Name` instead of rebuilding a `corev1.ObjectReference` |

Verification: `go vet -tags e2e ./test/e2e/...` and `go test -tags e2e -c ./test/e2e/`
both pass. (These confirm compilation only — the suite itself still requires
Docker/kind or AWS to run.)
