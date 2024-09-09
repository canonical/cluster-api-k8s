# Proposal information

<!-- Index number -->
- **Index**: 001

<!-- Status -->
- **Status**: **DRAFTING** <!-- **DRAFTING**/**ACCEPTED**/**REJECTED** -->

<!-- Short description for the feature -->
- **Name**: ClusterAPI In-Place Upgrades

<!-- Owner name and github handle -->
- **Owner**: Berkay Tekin Oz [@berkayoz](https://github.com/berkayoz) <!-- [@name](https://github.com/name) -->

# Proposal Details

## Summary
<!--
In a short paragraph, explain what the proposal is about and what problem
it is attempting to solve.
-->

Canonical Kubernetes CAPI providers should reconcile workload clusters and perform in-place upgrades based on the metadata in the cluster manifest.

This can be used in environments where rolling upgrades are not a viable option such as edge deployments and non-HA clusters.

## Rationale
<!--
This section COULD be as short or as long as needed. In the appropriate amount
of detail, you SHOULD explain how this proposal improves k8s-snap, what is the
problem it is trying to solve and how this makes the user experience better.

You can do this by describing user scenarios, and how this feature helps them.
You can also provide examples of how this feature may be used.
-->

The current Cluster API implementation does not provide a way of updating machines in-place and instead follows a rolling upgrade strategy. 

This means that a version upgrade would trigger a rolling upgrade, which is the process of creating new machines with desired configuration and removing older ones. This strategy is acceptable in most cases for clusters that are provisioned on public or private clouds where having extra resources is not a concern.

However this strategy is not viable for smaller bare-metal or edge deployments where resources are limited. This makes Cluster API not suitable out of the box for most of the use cases in industries like telco.

We can enable the use of Cluster API in these use-cases by updating our providers to perform in-place upgrades.


## User facing changes
<!--
This section MUST describe any user-facing changes that this feature brings, if
any. If an API change is required, the affected endpoints MUST be mentioned. If
the output of any k8s command changes, the difference MUST be mentioned, with a
clear example of "before" and "after".
-->

Users will be able to perform in-place upgrades per machine basis by running:
```sh
kubectl annotate machine <machine-name> v1beta2.k8sd.io/in-place-upgrade-to={upgrade-option}
```

`{upgrade-option}` can be one of:
* `channel=<channel>` which would refresh the machine to the provided channel e.g. `channel=1.31-classic/stable`
* `revision=<revision>` which would refresh the machine to the provided revision e.g. `revision=640`
* `localPath=<absolute-path-to-file>` which would refresh the machine to the provided local `*.snap` file e.g. `localPath=/path/to/k8s.snap`

## Alternative solutions
<!--
This section SHOULD list any possible alternative solutions that have been or
should be considered. If required, add more details about why these alternative
solutions were discarded.
-->

We could alternatively use the `version` fields defined in `CK8sControlPlane` and `MachineDeployment` manifests instead of annotations which could be a better/more native user experience.

However at the time of writing CAPI does not have support for changing upgrade strategies which means changes to the `version` fields trigger a rolling update.

This behaviour can be adjusted on `ControlPlane` objects as our provider has more/full control but can not be easily adjusted on `MachineDeployment` objects which causes issues for worker nodes.

Switching to using the `version` field should take place when upstream implements support for different upgrade strategies.

## Out of scope
<!--
This section MUST reference any work that is out of scope for this proposal.
Out of scope items are typically unknowns that we do not yet have a clear idea
of how to solve, so we explicitly do not tackle them until we have more
information.

This section is very useful to help guide the implementation details section
below, or serve as reference for future proposals.
-->

### Cluster-wide Orchestration
A cluster controller called `ClusterReconciler` is added which would perform the one-by-one in-place upgrade of the entire workload cluster. 

Users perform in-place upgrades on the entire cluster by running:
```sh
kubectl annotate cluster <cluster-name> v1beta2.k8sd.io/in-place-upgrade-to={upgrade-option}
```
This would upgrade machines belonging to `<cluster-name>` one by one.

The controller would propagate the `v1beta2.k8sd.io/in-place-upgrade-to` annotation on the `Cluster` object by adding this annotation one-by-one to all the machines that is owned by this cluster. 

The reconciler would perform upgrades in 2 separate phases for control-plane and worker machines. 

A Kubernetes API call listing the objects of type `Machine` and filtering with `ownerRef` would produce the list of machines owned by the cluster. For each phase controller would iterate over this list filtering by the machine type, annotating the machines and waiting for the operation to complete on each iteration.

The reconciler should not trigger the upgrade endpoint if `v1beta2.k8sd.io/in-place-upgrade-status` is already set to `in-progress` on the machine.

Once upgrades of the underlying machines are finished:
* `v1beta2.k8sd.io/in-place-upgrade-to` annotation on the `Cluster` would be removed
* `v1beta2.k8sd.io/in-place-upgrade-release` annotation on the `Cluster` would be added/updated with the used `{upgrade-option}`.

This process can be adapted to use `CK8sControlPlane` and `MachineDeployment` objects instead to be able to upgrade control-plane and worker nodes separately. This will be introduced and explained more extensively in another proposal.

### Upgrades of Underlying OS and Dependencies
The in-place upgrades only address the upgrades of Canonical Kubernetes and it's respective dependencies. Which means changes on the OS front/image would not be handled since the underlying machine image stays the same. This would be handled by a rolling upgrade as usual.

# Implementation Details

## API Changes
<!--
This section MUST mention any changes to the k8sd API, or any additional API
endpoints (and messages) that are required for this proposal.

Unless there is a particularly strong reason, it is preferable to add new v2/v3
APIs endpoints instead of breaking the existing APIs, such that API clients are
not affected.
-->
### `POST /snap/refresh`

```go
type SnapRefreshRequest struct {
	// Channel is the channel to refresh the snap to.
	Channel string `json:"channel"`
	// Revision is the revision number to refresh the snap to.
	Revision string `json:"revision"`
	// LocalPath is the local path to use to refresh the snap.
	LocalPath string `json:"localPath"`
}

// SnapRefreshResponse is the response message for the SnapRefresh RPC.
type SnapRefreshResponse struct {
	// The change id belonging to a snap refresh/install operation.
	ChangeID string `json:"changeId"`
}
```

`POST /snap/refresh` performs the in-place upgrade with the given options and returns the change id of the snap operation.

The upgrade can be either done with a `Channel`, `Revision` or a local `*.snap` file provided via `LocalPath`. The value of `LocalPath` should be an absolute path.

### `POST /snap/refresh-status`

```go
// SnapRefreshStatusRequest is the request message for the SnapRefreshStatus RPC.
type SnapRefreshStatusRequest struct {
	// The change id belonging to a snap refresh/install operation.
	ChangeID string `json:"changeId"`
}

// SnapRefreshStatusResponse is the response message for the SnapRefreshStatus RPC.
type SnapRefreshStatusResponse struct {
	// Status is the status of the snap refresh/install operation.
	Status string `json:"status"`
	// Completed is a boolean indicating if the snap refresh/install operation has completed.
	// The status should be considered final when this is true.
	Completed bool `json:"completed"`
	// ErrorMessage is the error message if the snap refresh/install operation failed.
	ErrorMessage string `json:"errorMessage"`
}
```
`POST /snap/refresh-status` returns the status of the refresh operation for the given change id.

The operation is considered fully complete once `Completed=true`.

The `Status` field will contain the status of the operation, with `Done` and `Error` being statuses of interest.

The `ErrorMessage` field is populated if the operation could not be completed successfully.


### Node Token Authentication 

A node token per node will be generated at bootstrap time, which gets seeded into the node under the `/capi/etc/node-token` file. On bootstrap the token under `/capi/etc/node-token` will be copied over to `/var/snap/k8s/common/node-token` with the help of `k8s x-capi set-node-token <token>` command. The generated token will be stored on the management cluster in the `$clustername-token` secret, with keys formatted as `refresh-token::$nodename`. 

The endpoints will use `ValidateNodeTokenAccessHandler("node-token")` to check the `node-token` header to match against the token in the `/var/snap/k8s/common/node-token` file.


## Bootstrap Provider Changes
<!--
This section MUST mention any changes to the bootstrap provider.
-->

A machine controller called `MachineReconciler` is added which would perform the in-place upgrade if `v1beta2.k8sd.io/in-place-upgrade-to` annotation is set on the machine.

The controller would use the value of this annotation to make an endpoint call to the `/snap/refresh` through `k8sd-proxy`. The controller then would periodically query the `/snap/refresh-status` with the change id of the operation until the operation is fully completed(`Completed=true`).

A failed request to `/snap/refresh` endpoint would requeue the requested upgrade without setting any annotations.

The result of the refresh operation will be communicated back to the user via the `v1beta2.k8sd.io/in-place-upgrade-status` annotation. Values being:

* `in-progress` for an upgrade currently in progress
* `done` for a successful upgrade
* `failed` for a failed upgrade

After an upgrade process begins:
* `v1beta2.k8sd.io/in-place-upgrade-status` annotation on the `Machine` would be added/updated with `in-progress`
* `v1beta2.k8sd.io/in-place-upgrade-change-id` annotation on the `Machine` would be updated with the change id returned from the refresh response.
* An `InPlaceUpgradeInProgress` event is added to the `Machine` with the `Performing in place upgrade with {upgrade-option}` message.

After a successfull upgrade:
* `v1beta2.k8sd.io/in-place-upgrade-to` annotation on the `Machine` would be removed
* `v1beta2.k8sd.io/in-place-change-id` annotation on the `Machine` would be removed
* `v1beta2.k8sd.io/in-place-upgrade-release` annotation on the `Machine` would be added/updated with the used `{upgrade-option}`.
* `v1beta2.k8sd.io/in-place-upgrade-status` annotation on the `Machine` would be added/updated with `done`
* An `InPlaceUpgradeDone` event is added to the `Machine` with the `Successfully performed in place upgrade with {upgrade-option}` message.

After a failed upgrade:
* `v1beta2.k8sd.io/in-place-upgrade-status` annotation on the `Machine` would be added/updated with `failed`
* `v1beta2.k8sd.io/in-place-change-id` annotation on the `Machine` would be removed
* An `InPlaceUpgradeFailed` event is added to the `Machine` with the `Failed to perform in place upgrade with option {upgrade-option}: {error}` message.

A custom condition with type `InPlaceUpgradeStatus` can also be added to relay these information. 

The reconciler should not trigger the upgrade endpoint if `v1beta2.k8sd.io/in-place-upgrade-status` is already set to `in-progress` on the machine.

#### Changes for Rolling Upgrades, Scaling Up and Creating New Machines
In case of a rolling upgrade or when creating new machines the `CK8sConfigReconciler` should check for the `v1beta2.k8sd.io/in-place-upgrade-release` annotation both on the `Machine` object.

The value of one of the annotation should be used instead of the `version` field while generating a cloud-init script for a machine.

Using an annotation value requires changing the `install.sh` file to perform the relevant snap operation based on the option.
* `snap install k8s --classic --channel <channel>` for `Channel`
* `snap install k8s --classic --revision <revision>` for `Revision`
* `snap install <path-to-snap> --classic --dangerous --name k8s` for `LocalPath`

When a rolling upgrade is triggered the `LocalPath` option requires the newly created machine to contain the local `*.snap` file. This usually means the machine image used by the infrastructure provider should be updated to contain this image. This file could possibly be sideloaded in the cloud-init script before installation.

This operation should not be performed if `install.sh` script is overridden by the user in the manifests.

This would prevent adding nodes with an outdated version and possibly breaking the cluster due to a version mismatch.

## ControlPlane Provider Changes
<!--
This section MUST mention any changes to the controlplane provider.
-->
none

## Configuration Changes
<!--
This section MUST mention any new configuration options or service arguments
that are introduced.
-->
none

## Documentation Changes
<!--
This section MUST mention any new documentation that is required for the new
feature. Most features are expected to come with at least a How-To and an
Explanation page.

In this section, it is useful to think about any existing pages that need to be
updated (e.g. command outputs).
-->
`How-To` page on performing in-place upgrades should be created.

`Reference` page listing the annotations and possible values should be created/updated.

## Testing
<!--
This section MUST explain how the new feature will be tested.
-->
The new feature can be tested manually by applying an annotation on the machine/node and checking for the `v1beta2.k8sd.io/in-place-upgrade-status` annotation's value to be `done`. A timeout should be set for waiting on the upgrade process.

The tests can be integrated into the CI the same way with the CAPD infrastructure provider.

The upgrade should be performed with the `localPath` option. Under Pebble the process would replace the `kubernetes` binary with the binary provided in the annotation value.

This means a docker image containing 2 versions should be created. The different/new version of the `kubernetes` binary would also be built and put into a path.

## Considerations for backwards compatibility
<!--
In this section, you MUST mention any breaking changes that are introduced by
this feature. Some examples:

- In case of deleting a database table, how do older k8sd instances handle it?
- In case of a changed API endpoint, how do existing clients handle it?
- etc
-->

## Implementation notes and guidelines
<!--
In this section, you SHOULD go into detail about how the proposal can be
implemented. If needed, link to specific parts of the code (link against
particular commits, not branches, such that any links remain valid going
forward).

This is useful as it allows the proposal owner to not be the person that
implements it.
-->

The annotation method is chosen due to the "immutable infrastructure" assumption CAPI currently has. Which means updates are always done by creating new machines and fields are immutable. This might also pose some challenges on displaying accurate Kubernetes version information through CAPI.

We should be aware of the [metadata propagation](https://cluster-api.sigs.k8s.io/developer/architecture/controllers/metadata-propagation) performed by the upstream controllers. Some metadata is propagated in-place, which can ultimately propagate all the way down to the `Machine` objects. This could potentially flood the cluster with upgrades if machines get annotated at the same time. The cluster wide upgrade is handled through the annotation on the actual Cluster object due to this reason.

Updating the `version` field would trigger rolling updates by default, with the only difference than upstream being the precedence of the version value provided in the annotations. 
