# Proposal information

<!-- Index number -->
- **Index**: 002

<!-- Status -->
- **Status**: **DRAFTING**

<!-- Short description for the feature -->
- **Name**: Cluster-wide In-place Upgrades

<!-- Owner name and github handle -->
- **Owner**: [Homayoon (Hue) Alimohammadi](https://www.github.com/HomayoonAlimohammadi)

# Proposal Details

## Summary
<!--
In a short paragraph, explain what the proposal is about and what problem
it is attempting to solve.
-->

According to [the in-place upgrade proposal], we have the ability to start an in-place upgrade for a single machine by setting an annotation like [InPlaceUpgradeToAnnotation] which will trigger the in-place upgrade for that specific machine. Performing an in-place upgrade for the whole cluster however, requires the cluster admin to manually apply this annotation on each and every machine, making this process a good candidate for automation.
In this proposal we describe the process of adding a `ClusterReconciler` which is responsible for watching for the [InPlaceUpgradeToAnnotation] on the cluster and will automatically perform in-place upgrades for every single machine in the cluster.

## Rationale
<!--
This section COULD be as short or as long as needed. In the appropriate amount
of detail, you SHOULD explain how this proposal improves cluster-api-k8s, what is the
problem it is trying to solve and how this makes the user experience better.

You can do this by describing user scenarios, and how this feature helps them.
You can also provide examples of how this feature may be used.
-->

Currently, cluster-wide rolling upgrades are supported natively by CAPI. By changing the `version` field of control plane or worker nodes, one is able to effortlessly upgrade the kubernetes version of the whole cluster in a rolling manner. Having a dedicated controller in charge of the cluster-wide in-place upgrades is intended to facilitate this process and gracefully handle any failure that might occur. 


## User facing changes
<!--
This section MUST describe any user-facing changes that this feature brings, if
any. If an API change is required, the affected endpoints MUST be mentioned. If
the output of any k8s command changes, the difference MUST be mentioned, with a
clear example of "before" and "after".
-->

Users can annotation the provisioned (workload) cluster with [InPlaceUpgradeToAnnotation] which cause each machine to undergo an in-place upgrade orchestrated by the `ClusterReconciler`.

## Alternative solutions
<!--
This section SHOULD list any possible alternative solutions that have been or
should be considered. If required, add more details about why these alternative
solutions were discarded.
-->

- Support for single machine and cluster-wide in-place upgrade can also be added to the upstream CAPI.

## Out of scope
<!--
This section MUST reference any work that is out of scope for this proposal.
Out of scope items are typically unknowns that we do not yet have a clear idea
of how to solve, so we explicitly do not tackle them until we have more
information.

This section is very useful to help guide the implementation details section
below, or serve as reference for future proposals.
-->

We could have a mechanism to intelligently spot unallowed version skews and impossible upgrades.
Also, according to the current proposal, once the annotation is observed on the cluster, `ClusterReconciler` is responsible for performing the in-place upgrade to completion. Rollback mechanisms might be needed in case of failure or if the admin decides to not proceed with the upgrade (e.g. by removing the [InPlaceUpgradeToAnnotation]).

# Implementation Details

## API Changes
<!--
This section MUST mention any changes to the k8sd API, or any additional API
endpoints (and messages) that are required for this proposal.

Unless there is a particularly strong reason, it is preferable to add new v2/v3
APIs endpoints instead of breaking the existing APIs, such that API clients are
not affected.
-->

none

## Bootstrap Provider Changes
<!--
This section MUST mention any changes to the bootstrap provider.
-->
none

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

The implemented solution should have detailed documentation on:
- An explanation on the `ClusterReconciler` inner workings
- A how to guide on doing a cluster-wide in-place upgrade
- A reference page on cluster object annotations and their purposes:
    - [InPlaceUpgradeToAnnotation]
    - [InPlaceUpgradeStatusAnnotation]
    - [InPlaceUpgradeReleaseAnnotation]

## Testing
<!--
This section MUST explain how the new feature will be tested.
-->

- Unit tests for the `ClusterReconciler` reconciliation loop and any other added logic
- End-to-end tests for cluster-wide in-place upgrades that confirm desired behavior in happy and failure scenarios:
    - Failure scenarios include: 
        - Failed upgrade on a single machine

## Considerations for backwards compatibility
<!--
In this section, you MUST mention any breaking changes that are introduced by
this feature.
-->

none

## Implementation notes and guidelines
<!--
In this section, you SHOULD go into detail about how the proposal can be
implemented. If needed, link to specific parts of the code (link against
particular commits, not branches, such that any links remain valid going
forward).

This is useful as it allows the proposal owner to not be the person that
implements it.
-->

A `ClusterReconciler` struct is created in the [bootstrap/cluster_controller.go] with (at least) the `Reconcile` and `SetupWithManager` methods.
```go
type ClusterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
    recorder record.EventRecorder

	machineGetter ck8s.MachineGetter
}

func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ reconcile.Result, rerr error) {
    ...
}

func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
    ...
}
```
Since we only need to get machines (cp and worker) for the cluster, we don't need a `ManagementCluster` and should instead define and use the `MachineGetter` interface in [pkg/ck8s/machine_getter.go] to be used in `ClusterReconciler`:
```go
type MachineGetter interface {
    GetMachinesForCluster(ctx context.Context, cluster client.ObjectKey, filters ...collections.Func) (collections.Machines, error)
}
```
This reconciler should get registered in the [bootstrap/main.go]:
```go
if err = (&controllers.ClusterReconciler{
    Client: mgr.GetClient(),
    Log:    ctrl.Log.WithName("controllers").WithName("Cluster"),
    Scheme: mgr.GetScheme(),
}).SetupWithManager(mgr); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "Cluster")
    os.Exit(1)
}
```
In the `Reconcile()` function, the reconciler should watch the [InPlaceUpgradeToAnnotation] and upon the annotation creation, it should do the following:
- Annotate the cluster object with [InPlaceUpgradeStatusAnnotation]: [InPlaceUpgradeInProgressStatus]
- Get the controll plane nodes (machines)
    - Annotate the first one with [InPlaceUpgradeToAnnotation]
    - Wait for that machine to get annotated with [InPlaceUpgradeStatusAnnotation]: [InPlaceUpgradeDoneStatus] which indicates success or [InPlaceUpgradeStatusAnnotation]: [InPlaceUpgradeFailedStatus] which indicates failure
    - In case of failure, annotate the cluster with [InPlaceUpgradeStatusAnnotation]: [InPlaceUpgradeFailedStatus] and remove the [InPlaceUpgradeToAnnotation].
    - In case of success repeat the same process for every other control plane node
- Get the worker nodes (machines)
    - Repeat the same process for worker nodes sequantially. 
- If all the machine upgrades were successful, annotation the cluster with [InPlaceUpgradeStatusAnnotation]: [InPlaceUpgradeDoneStatus] and [InPlaceUpgradeReleaseAnnotation] and remove the [InPlaceUpgradeToAnnotation]

`NOTE`: sequantial upgrades will help with rollback and recovery in case of failures and also prevent downtimes on the workload cluster since nodes are not getting restarted all at the same time.

<!-- LINKS -->
[the in-place upgrade proposal]: https://github.com/canonical/cluster-api-k8s/pull/30
[InPlaceUpgradeToAnnotation]: ../../bootstrap/api/v1beta2/inplace_upgrade_consts.go#L4
[InPlaceUpgradeStatusAnnotation]: ../../bootstrap/api/v1beta2/inplace_upgrade_consts.go#L5
[InPlaceUpgradeReleaseAnnotation]: ../../bootstrap/api/v1beta2/inplace_upgrade_consts.go#L6
[InPlaceUpgradeInProgressStatus]: ../../bootstrap/api/v1beta2/inplace_upgrade_consts.go#L11
[InPlaceUpgradeDoneStatus]: ../../bootstrap/api/v1beta2/inplace_upgrade_consts.go#L12
[InPlaceUpgradeFailedStatus]: ../../bootstrap/api/v1beta2/inplace_upgrade_consts.go#L13
[bootstrap/cluster_controller.go]: ../../bootstrap/cluster_controller.go
[pkg/ck8s/machine_getter.go]: ../../pkg/ck8s/machine_getter.go
[bootstrap/main.go]: ../../bootstrap/main.go
