package inplace

import (
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
)

// IsUpgraded checks if the object is already upgraded to the specified release.
func IsUpgraded(obj client.Object, release string) bool {
	return obj.GetAnnotations()[bootstrapv1.InPlaceUpgradeReleaseAnnotation] == release
}

// GetUpgradeInstructions returns the in-place upgrade instructions set on the object.
func GetUpgradeInstructions(obj client.Object) string {
	// NOTE(Hue): The reason we are checking the `release` annotation as well is that we want to make sure
	// we upgrade the new machines that joined after the initial upgrade process.
	// The `upgrade-to` overwrites the `release` annotation, because we might have both in case
	// the user decides to do another in-place upgrade after a successful one.
	upgradeTo := obj.GetAnnotations()[bootstrapv1.InPlaceUpgradeReleaseAnnotation]
	if to, ok := obj.GetAnnotations()[bootstrapv1.InPlaceUpgradeToAnnotation]; ok {
		upgradeTo = to
	}

	return upgradeTo
}

// IsMachineUpgradeFailed checks if the machine upgrade failed.
// The upgrade might be getting retried at the moment of the check. This check insures that the upgrade failed *at some point*.
func IsMachineUpgradeFailed(m *clusterv1.Machine) bool {
	return m.Annotations[bootstrapv1.InPlaceUpgradeLastFailedAttemptAtAnnotation] != ""
}

// IsMachineUpgrading checks if the object is upgrading.
func IsMachineUpgrading(m *clusterv1.Machine) bool {
	// NOTE(Hue): We can't easily generalize this function to check for all objects.
	// Generally speaking, the `status == in-progress` should indicate that the object is upgrading.
	// But from the orchestrated upgrade perspective, we need to also check the `upgrade-to` annotation
	// so that we know if the single machine inplace upgrade
	// controller is going to handle the upgrade process, hence "in-progress".
	return m.Annotations[bootstrapv1.InPlaceUpgradeStatusAnnotation] == bootstrapv1.InPlaceUpgradeInProgressStatus ||
		m.Annotations[bootstrapv1.InPlaceUpgradeToAnnotation] != ""
}
