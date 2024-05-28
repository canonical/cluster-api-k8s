/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

// Conditions and condition Reasons for the KThreesConfig object.

const (
	// DataSecretAvailableCondition documents the status of the bootstrap secret generation process.
	//
	// NOTE: When the DataSecret generation starts the process completes immediately and within the
	// same reconciliation, so the user will always see a transition from Wait to Generated without having
	// evidence that BootstrapSecret generation is started/in progress.
	DataSecretAvailableCondition clusterv1.ConditionType = "DataSecretAvailable"

	// WaitingForClusterInfrastructureReason (Severity=Info) document a bootstrap secret generation process
	// waiting for the cluster infrastructure to be ready.
	//
	// NOTE: Having the cluster infrastructure ready is a pre-condition for starting to create machines;
	// the KThreesConfig controller ensure this pre-condition is satisfied.
	WaitingForClusterInfrastructureReason = "WaitingForClusterInfrastructure"

	// WaitingForControlPlaneAvailableReason (Severity=Info) document a bootstrap secret generation process
	// waiting for the control plane machine to be available.
	//
	// NOTE: Having the control plane machine available is a pre-condition for joining additional control planes
	// or workers nodes.
	// Deprecated: This has been deprecated in v1beta1 and will be removed in a future version.
	// Switch to WaitingForControlPlaneAvailableReason constant from the `sigs.k8s.io/cluster-api/api/v1beta1`
	// package.
	WaitingForControlPlaneAvailableReason = clusterv1.WaitingForControlPlaneAvailableReason

	// DataSecretGenerationFailedReason (Severity=Warning) documents a KThreesConfig controller detecting
	// an error while generating a data secret; those kind of errors are usually due to misconfigurations
	// and user intervention is required to get them fixed.
	DataSecretGenerationFailedReason = "DataSecretGenerationFailed"
)

const (
	// CertificatesAvailableCondition documents that cluster certificates are available.
	//
	// NOTE: Cluster certificates are generated only for the KThreesConfig object linked to the initial control plane
	// machine, if the cluster is not using a control plane ref object, if the certificates are not provided
	// by the users.
	// IMPORTANT: This condition won't be re-created after clusterctl move.
	CertificatesAvailableCondition clusterv1.ConditionType = "CertificatesAvailable"

	// CertificatesGenerationFailedReason (Severity=Warning) documents a KThreesConfig controller detecting
	// an error while generating certificates; those kind of errors are usually temporary and the controller
	// automatically recover from them.
	CertificatesGenerationFailedReason = "CertificatesGenerationFailed"

	// CertificatesCorruptedReason (Severity=Error) documents a KThreesConfig controller detecting
	// an error while retrieving certificates for a joining node.
	CertificatesCorruptedReason = "CertificatesCorrupted"
)
