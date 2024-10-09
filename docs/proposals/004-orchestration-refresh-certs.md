<!--
To start a new proposal, create a copy of this template on this directory and
fill out the sections below.
-->

# Proposal information

<!-- Index number -->
- **Index**: 004

<!-- Status -->
- **Status**: ACCEPTED
<!-- **DRAFTING**/**ACCEPTED**/**REJECTED** -->

<!-- Short description for the feature -->
- **Name**: Cluster Orchestration - Certificate Refresh

<!-- Owner name and github handle -->
- **Owner**: Mateo Florido [@mateoflorido](https://github.com/mateoflorido)

# Proposal Details

## Summary
<!--
In a short paragraph, explain what the proposal is about and what problem
it is attempting to solve.
-->

This proposal aims to introduce a mechanism to refresh certificates for all
nodes in a Canonical Kubernetes CAPI cluster simultaneously, removing the
need to annotate each machine individually. This feature will allow the
administrators to trigger a cluster-wide certificate refresh through
annotations on higher-level resources like `Cluster`, `CK8sControlPlane`, or
`MachineDeployment`.

## Rationale
<!--
This section COULD be as short or as long as needed. In the appropriate amount
of detail, you SHOULD explain how this proposal improves k8s providers, what is the
problem it is trying to solve and how this makes the user experience better.

You can do this by describing user scenarios, and how this feature helps them.
You can also provide examples of how this feature may be used.
-->

We currently have the ability to refresh the certificates for individual
`Machine` resources in the cluster. However, this process can be time consuming
as it requires annotating each `Machine` resource individually and waiting for
the certificates to refresh. In this proposal, we aim to introduce the
capability to refresh certificates for all nodes in the cluster at once. This
new feature will improve the user experience and speed up the process,
especially in large clusters.

## User facing changes
<!--
This section MUST describe any user-facing changes that this feature brings, if
any. If an API change is required, the affected endpoints MUST be mentioned. If
the output of any k8s command changes, the difference MUST be mentioned, with a
clear example of "before" and "after".
-->

Administrators can annotate the `Cluster`, `CK8sControlPlane` or
`MachineDeployment` objects to trigger the certificate refresh for machines in the
cluster, control plane nodes, or worker nodes, respectively.

```yaml
kubectl annotate cluster <name> v1beta2.k8sd.io/refresh-certificates={expires-in}
kubectl annotate ck8scontrolplane <name> v1beta2.k8sd.io/refresh-certificates={expires-in}
kubectl annotate machinedeployment <name> v1beta2.k8sd.io/refresh-certificates={expires-in}
```

`expires-in` specifies how long the certificates will be valid. It can be
expressed in years, months, days, or other time units supported by the
`time.ParseDuration` function.

## Alternative solutions
<!--
This section SHOULD list any possible alternative solutions that have been or
should be considered. If required, add more details about why these alternative
solutions were discarded.
-->

As mentioned in the [Proposal 003], the Kubeadm Control Plane Provider can
refresh the certificates for the nodes in the cluster. However, this approach
requires performing a rolling update of the machines owned by the cluster.

## Out of scope
<!--
This section MUST reference any work that is out of scope for this proposal.
Out of scope items are typically unknowns that we do not yet have a clear idea
of how to solve, so we explicitly do not tackle them until we have more
information.

This section is very useful to help guide the implementation details section
below, or serve as reference for future proposals.
-->

This proposal does not include the functionality to refresh certificates via
a rolling update of nodes or automatically trigger the process when
certificates are close to expiring. Aditionally, it does not cover
the renewing of external certificates provided by the user or CA certificates.

# Implementation Details

## API Changes
<!--
This section MUST mention any changes to the k8sd API, or any additional API
endpoints (and messages) that are required for this proposal.

Unless there is a particularly strong reason, it is preferable to add new v2/v3
APIs endpoints instead of breaking the existing APIs, such that API clients are
not affected.
-->

None

## Bootstrap Provider Changes
<!--
This section MUST mention any changes to the bootstrap provider.
-->

### Cluster Controller

We will add a new controller, `ClusterCertificatesReconciler`, to the bootstrap
provider. This controller will monitor for `Cluster` objects and trigger a
certificate refresh for all nodes in the cluster when the
`v1beta2.k8sd.io/refresh-certificates` annotation is applied.

The status of the certificate refresh process will be shared via the `Cluster`
object by emitting events. The events that the controller can emit are:
- `RefreshCertsInProgress`: The certificate refresh process has started.
- `RefreshCertsDone`: The certificate refresh process has finished successfully.
- `RefreshCertsFailed`: The certificate refresh process has failed.

The controller should perform the following steps:
1. Retrieve the `CK8sControlPlane` object owned by the `Cluster` object.
2. Emit the `RefreshCertsInProgress` event for the `Cluster` object.
3. Trigger a certificate refresh for the control plane nodes by annotating the
   `CK8sControlPlane` object with the `v1beta2.k8sd.io/refresh-certificates`
   annotation.
4. Wait for the certificates to be refreshed on the control plane nodes. The
   controller should check the `v1beta2.k8sd.io/refresh-certificates-status`
   annotation to determine when the certificates have been refreshed.
5. If the refresh is successful, the controller proceeds to the
   `MachineDeployment` objects.
6. For each `MachineDeployment` object, trigger a certificate refresh for the
   worker nodes by annotating the `MachineDeployment` object with the
   `v1beta2.k8sd.io/refresh-certificates` annotation.
7. Wait for the certificates to be refreshed on the worker nodes, checking the
   `v1beta2.k8sd.io/refresh-certificates-status` annotation.
8. If the refresh is successful, the controller emits the `RefreshCertsDone`
   event for the `Cluster` object.

### MachineDeployment Controller

We also need to add a new controller, `MachineDeployCertificatesReconciler`, to
the bootstrap provider. This controller will watch for `MachineDeployment`
objects and trigger a certificate refresh for all the worker nodes in the
cluster when the `v1beta2.k8sd.io/refresh-certificates` annotation is present.

The controller should perform the following steps:
1. List all machines owned by the `MachineDeployment` object and filter out the
   control plane machines.
2. Emit the `RefreshCertsInProgress` event for the `MachineDeployment` object.
3. For each machine, trigger the certificate refresh by annotating the machine
   with the `v1beta2.k8sd.io/refresh-certificates` annotation.
4. Wait for the certificates to be refreshed on that machine. The controller
   should check the `v1beta2.k8sd.io/refresh-certificates-status` annotation
   to know when the certificates have been refreshed.
5. If the refresh is successful, the controller moves to the next machine.

The status of the certificate refresh process will be shared via the
`MachineDeployment` object in the same way as the `Cluster` controller.

## ControlPlane Provider Changes
<!--
This section MUST mention any changes to the controlplane provider.
-->

A controller `ControlPlaneCertificatesReconciler` will be added to the control plane
provider. This controller will watch for the `CK8sControlPlane` objects and
will trigger the certificate refresh for all the control plane nodes in the
cluster when the `v1beta2.k8sd.io/refresh-certificates` annotation is present.

The controller should perform the following steps:
1. List all the control plane machines owned by the `CK8sControlPlane` object.
2. Emit the `RefreshCertsInProgress` event for the `CK8sControlPlane` object.
3. For each control plane machine, trigger the certificate by annotating the
   machine with the `v1beta2.k8sd.io/refresh-certificates` annotation.
4. Wait for the certificates to be refreshed in that machine. The controller
   should check the `v1beta2.k8sd.io/refresh-certificates-status`
   annotation to know when the certificates have been refreshed.
5. If the upgrade is sucessful, the controller moves to the next machine.
   If the upgrade fails, the controller emits the `RefreshCertsFailed` event
   for the `CK8sControlPlane` object and stops the process.
6. Once all the control plane machines have been refreshed, the controller emits
   the `RefreshCertsDone` event for the `CK8sControlPlane` object.

As mentioned in the Bootstrap Provider Changes, the status of the certificate
refresh process will be shared via the `CK8sControlPlane` object. Using the
same events as the `Cluster` controller.

## Configuration Changes
<!--
This section MUST mention any new configuration options or service arguments
that are introduced.
-->

None

## Documentation Changes
<!--
This section MUST mention any new documentation that is required for the new
feature. Most features are expected to come with at least a How-To and an
Explanation page.

In this section, it is useful to think about any existing pages that need to be
updated (e.g. command outputs).
-->
This proposal will require a new section in the Canonical Kubernetes
documentation explaining:
- How-to page on how refresh the certificates for a cluster.
- Explanation page on how the refreshing orchestration process works.

## Testing
<!--
This section MUST explain how the new feature will be tested.
-->

- Unit tests for the controllers in the bootstrap and control plane providers.
- Integration tests which cover the refreshing process for the certificates in
  the cluster using the `v1beta2.k8sd.io/refresh-certificates` annotation in
  the `Cluster`, `CK8sControlPlane` and `MachineDeployment` objects.

## Considerations for backwards compatibility
<!--
In this section, you MUST mention any breaking changes that are introduced by
this feature. Some examples:

- In case of deleting a database table, how do older k8sd instances handle it?
- In case of a changed API endpoint, how do existing clients handle it?
- etc
-->

None

## Implementation notes and guidelines
<!--
In this section, you SHOULD go into detail about how the proposal can be
implemented. If needed, link to specific parts of the code (link against
particular commits, not branches, such that any links remain valid going
forward).

This is useful as it allows the proposal owner to not be the person that
implements it.
-->
For this implementation, we can take as a reference the implementation of the
`CertificateController` in our repository. This controller is responsible for
refreshing the certificates for the Machines in the cluster. We are going to
leverage the logic in there to offload the certificate refresh process to this
controller.

<!-- Links -->

[Proposal 003]: 003-refresh-certs.md

