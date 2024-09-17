<!--
To start a new proposal, create a copy of this template on this directory and
fill out the sections below.
-->

# Proposal information

<!-- Index number -->
- **Index**: 003

<!-- Status -->
- **Status**: **DRAFTING**
<!-- **DRAFTING**/**ACCEPTED**/**REJECTED** -->

<!-- Short description for the feature -->
- **Name**: ClusterAPI Certificates Refresh

<!-- Owner name and github handle -->
- **Owner**: Mateo Florido [@mateoflorido](https://github.com/mateoflorido)
<!-- [@name](https://github.com/name) -->

# Proposal Details

## Summary
<!--
In a short paragraph, explain what the proposal is about and what problem
it is attempting to solve.
-->

The proposal aims to enhance Canonical Kubernetes Cluster API Providers by
enabling administrators to refresh or renew certificates on cluster nodes
without the need for a rolling upgrade. This feature is particularly beneficial
in resource-constrained environments, such as private or edge clouds, where
performing a full node replacement may not be feasible.

## Rationale
<!--
This section COULD be as short or as long as needed. In the appropriate amount
of detail, you SHOULD explain how this proposal improves k8s providers, what is the
problem it is trying to solve and how this makes the user experience better.

You can do this by describing user scenarios, and how this feature helps them.
You can also provide examples of how this feature may be used.
-->

Currently, Cluster API lacks a mechanism for refreshing certificates on cluster
nodes without triggering a full rolling update. For example, while the Kubeadm
provider offers the ability to renew certificates, it requires a rolling update
of the cluster nodes or manual intervention before the certificates expire.

This proposal aims to address this gap by enabling certificate renewal on
cluster nodes without requiring a rolling update. By providing administrators
with the ability to refresh certificates independently of node upgrades, this
feature improves cluster operation, especially in environments with limited
resources, such as private or edge clouds.

It will enhance the user experience by minimizing downtime, reducing the need
for additional resources, and simplifying certificate management. This is
particularly valuable for users who need to maintain continuous availability
or operate in environments where rolling updates are not practical due to
resource constraints.


## User facing changes
<!--
This section MUST describe any user-facing changes that this feature brings, if
any. If an API change is required, the affected endpoints MUST be mentioned. If
the output of any k8s command changes, the difference MUST be mentioned, with a
clear example of "before" and "after".
-->

Administrators will be able to renew certificates on cluster nodes without
triggering a full rolling update. This can be achieved by annotating the Machine
object, which will initiate the certificate renewal process:

```
kubectl annotate machine <machine-name> v1beta2.k8sd.io/refresh-certificates={expires-in}
```

`expires-in` specifies how long the certificate will remain valid. It can be
expressed in years, months, days, additionally to other time units supported by
the `time.ParseDuration`.

For tracking the validity of certificates, the Machine object will include a
`machine.cluster.x-k8s.io/certificates-expiry` annotation that indicates the
expiry date of the certificates. This annotation will be added when the cluster
is deployed and updated when certificates are renewed. The value of this
annotation will be a RFC3339 timestamp.

## Alternative solutions
<!--
This section SHOULD list any possible alternative solutions that have been or
should be considered. If required, add more details about why these alternative
solutions were discarded.
-->

**Kubeadm Control Plane provider (KCP)** automates certificate rotations for
control plane machines by triggering a machine rollout when certificates are
close to expiration.

### How to configure:
- In the KCP configuration, set the `rolloutBefore.certificatesExpiryDays`
field. This tells KCP when to trigger the rollout before certificates expire:

```yaml
spec:
  rolloutBefore:
    certificatesExpiryDays: 21  # Trigger rollout when certificates expire within 21 days
```

### How it works:
- **Automatic Rollouts**: KCP monitors the certificate expiry dates of control
plane machines using the `Machine.Status.CertificatesExpiryDate`. If
certificates are about to expire (based on a configured threshold), KCP
triggers a machine rollout to refresh them.
- **Certificate Expiry Check**: The expiry date is sourced from the
`machine.cluster.x-k8s.io/certificates-expiry` annotation on the Machine or
Bootstrap Config object.

For manual rotations, the administrator should run the `kubeadm certs renew`
command, ensure all control plane components are restarted, and remove the
expiry annotation for KCP to detect the updated certificate expiry date.


## Out of scope
<!--
This section MUST reference any work that is out of scope for this proposal.
Out of scope items are typically unknowns that we do not yet have a clear idea
of how to solve, so we explicitly do not tackle them until we have more
information.

This section is very useful to help guide the implementation details section
below, or serve as reference for future proposals.
-->

This proposal does not cover the orchestration of certificate renewal for the
whole cluster. It focuses on renewing certificates on individual cluster nodes
via the Machine object.

Rolling updates of the cluster nodes are out of scope. This proposal aims to
renew certificates without triggering a full rolling update of the cluster.

External certificate authorities (CAs) are also out of scope. This proposal
focuses on renewing self-signed certificates generated by Canonical Kubernetes.

# Implementation Details

## API Changes
<!--
This section MUST mention any changes to the k8sd API, or any additional API
endpoints (and messages) that are required for this proposal.

Unless there is a particularly strong reason, it is preferable to add new v2/v3
APIs endpoints instead of breaking the existing APIs, such that API clients are
not affected.
-->

### `GET /k8sd/certificates-expiry`

This endpoint will return the expiry date of the certificates on a specific
cluster node. The response will include the expiry date of the certificates
in RFC3339 format. The value will be sourced from the Kubernetes API server
certificate.

```go
type CertificatesExpiryResponse struct {
  // ExpiryDate is the expiry date of the certificates on the node.
  ExpiryDate string `json:"expiry-date"`
}
```

### `POST /x/capi/request-certificates`

This endpoint will create the necessary Certificate Signing Request (CSR) for
a worker node. The request will include the duration after which the
certificates will expire.

```go
type RequestCertificatesRequest struct {
  // ExpirationSeconds is the duration after which the certificates will expire.
  ExpirationSeconds int `json:"expiration-seconds"`
}
```

### `POST /x/capi/refresh-certificates`

This endpoint will trigger the renewal of certificates on a specific node.
The request will include the duration after which the certificates will expire
and a list of additional Subject Alternative Names (SANs) to include in the
certificate.

This endpoint is applicable to both control plane and worker nodes. For worker
nodes, the request will include the seed used to generate the CSR.

```go
type RefreshCertificatesRequest struct {
  // Seed is the seed used to generate the CSR.
  Seed string `json:"seed"`
  // ExpirationSeconds is the duration after which the certificates will expire.
  ExpirationSeconds int `json:"expiration-seconds"`
  //ExtraSANs is a list of additional Subject Alternative Names to include in the certificate.
  ExtraSANs []string `json:"extra-sans"`
}
```

### `POST /x/capi/approve-certificates`

This endpoint will approve the renewal of certificates for a worker node and
will be run by a control plane node. The request will include the seed used to
generate the CSR.

```go
type ApproveCertificatesRequest struct {
  // Seed is the seed used to generate the CSR.
  Seed string `json:"seed"`
}
```

## Bootstrap Provider Changes
<!--
This section MUST mention any changes to the bootstrap provider.
-->

A controller called `CertificatesController` will be added to the bootstrap
provider. This controller will watch for the `v1beta2.k8sd.io/refresh-certificates`
annotation on the Machine object and trigger the certificate renewal process
when the annotation is present.

### Control Plane Nodes

The controller would use the value of the
`v1beta2.k8sd.io/refresh-certificates`annotation to determine the duration
after which the certificates will expire. It will then call the
`POST /x/capi/refresh-certificates` endpoint to trigger the certificate
renewal process.

The controller will share the status of the certificate renewal process by
adding events to the Machine object. The events will indicate the progress of
the renewal process following this pattern:

- `RefreshCertsInProgress`: The certificate renewal process is in progress, the
  event will include the `Refreshing certificates in progress` message.
- `RefreshCertsDone`: The certificate renewal process is complete, the event
  will include the `Certificates have been refreshed` message.
- `RefreshCertsFailed`: The certificate renewal process has failed, the event
  will include the `Certificates renewal failed: {reason}` message.

After the certificate renewal process is complete, the controller will update
the `machine.cluster.x-k8s.io/certificates-expiry` annotation on the Machine
object with the new expiry date of the certificates.

Finally, the controller will remove the `v1beta2.k8sd.io/refresh-certificates`
annotation from the Machine object to indicate that the certificate renewal
process is complete.

### Worker Nodes

The controller would use the value of the `k8sd.io/refresh-certificates`
annotation to determine the duration after which the certificates will expire.
It will then call the `POST /x/capi/request-certificates` endpoint to create
the Certificate Signing Request (CSR) for the worker node.

Using the `k8sd` proxy, the controller can call the
`POST /x/capi/approve-certificates` endpoint with the seed generated in the
previous step to approve the CSRs for the worker node.

The controller will share the status similar to the control plane nodes by
emitting events to the `Machine` object. The events will indicate the progress
of the renewal process following the same pattern as in the control plane
nodes.

After the CSR approval process is complete, the worker node will call the
`POST /x/capi/refresh-certificates` endpoint to trigger the certificate renewal
process, using the seed generated to recover the certificates from the CSR
resources.

After the certificate renewal process is complete, the controller will update
the `machine.cluster.x-k8s.io/certificates-expiry` annotation on the Machine
object with the new expiry date of the certificates.

Finally, the controller will remove the `v1beta2.k8sd.io/refresh-certificates`
annotation
from the Machine object to indicate that the certificate renewal process is
complete.

## ControlPlane Provider Changes
<!--
This section MUST mention any changes to the controlplane provider.
-->

None

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

This implementation will require adding the following documentation:
- How-to guide for renewing certificates on cluster nodes
- Reference page of the `v1beta2.k8sd.io/refresh-certificates` annotation

## Testing
<!--
This section MUST explain how the new feature will be tested.
-->

Integration tests will be added to the current test suite. The tests will
create a cluster, annotate the Machine object with the
`v1beta2.k8sd.io/refresh-certificates` annotation, and verify that the
certificates are renewed in the target node.

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

We can leverage the existing certificate renewal logic in the k8s-snap.
For worker nodes, we need to modify the exisiting code to avoid blocking
the request until the certificates have been approved and issued. Instead,
we can use a multiple step process. Generating the CSRs, approving them, and
then trigger the certificate renewal process.

