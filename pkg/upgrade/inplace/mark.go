package inplace

import (
	"context"
	"fmt"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
)

// MarkMachineToUpgrade marks the machine to upgrade.
func MarkMachineToUpgrade(ctx context.Context, m *clusterv1.Machine, to string, c client.Client) error {
	patchHelper, err := patch.NewHelper(m, c)
	if err != nil {
		return fmt.Errorf("failed to create new patch helper: %w", err)
	}

	if m.Annotations == nil {
		m.Annotations = make(map[string]string)
	}

	// clean up
	delete(m.Annotations, bootstrapv1.InPlaceUpgradeReleaseAnnotation)
	delete(m.Annotations, bootstrapv1.InPlaceUpgradeStatusAnnotation)
	delete(m.Annotations, bootstrapv1.InPlaceUpgradeChangeIDAnnotation)
	delete(m.Annotations, bootstrapv1.InPlaceUpgradeLastFailedAttemptAtAnnotation)

	m.Annotations[bootstrapv1.InPlaceUpgradeToAnnotation] = to

	if err := patchHelper.Patch(ctx, m); err != nil {
		return fmt.Errorf("failed to patch: %w", err)
	}

	return nil
}

// MarkUpgradeFailed annotates the object with in-place upgrade failed.
func MarkUpgradeFailed(ctx context.Context, obj client.Object, patcher Patcher) error {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// clean up
	delete(annotations, bootstrapv1.InPlaceUpgradeReleaseAnnotation)

	annotations[bootstrapv1.InPlaceUpgradeStatusAnnotation] = bootstrapv1.InPlaceUpgradeFailedStatus
	obj.SetAnnotations(annotations)

	if err := patcher.Patch(ctx, obj); err != nil {
		return fmt.Errorf("failed to patch: %w", err)
	}

	return nil
}

// MarkUpgradeInProgress annotates the object with in-place upgrade in-progress.
func MarkUpgradeInProgress(ctx context.Context, obj client.Object, to string, patcher Patcher) error {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// clean up
	delete(annotations, bootstrapv1.InPlaceUpgradeReleaseAnnotation)

	annotations[bootstrapv1.InPlaceUpgradeStatusAnnotation] = bootstrapv1.InPlaceUpgradeInProgressStatus
	annotations[bootstrapv1.InPlaceUpgradeToAnnotation] = to

	obj.SetAnnotations(annotations)

	if err := patcher.Patch(ctx, obj); err != nil {
		return fmt.Errorf("failed to patch: %w", err)
	}

	return nil
}

// MarkUpgradeDone annotates the object with in-place upgrade done.
func MarkUpgradeDone(ctx context.Context, obj client.Object, to string, patcher Patcher) error {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// clean up
	delete(annotations, bootstrapv1.InPlaceUpgradeToAnnotation)

	annotations[bootstrapv1.InPlaceUpgradeStatusAnnotation] = bootstrapv1.InPlaceUpgradeDoneStatus
	annotations[bootstrapv1.InPlaceUpgradeReleaseAnnotation] = to

	obj.SetAnnotations(annotations)

	if err := patcher.Patch(ctx, obj); err != nil {
		return fmt.Errorf("failed to patch: %w", err)
	}

	return nil
}
