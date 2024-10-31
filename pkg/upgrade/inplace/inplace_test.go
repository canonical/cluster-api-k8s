package inplace_test

import (
	"testing"
	"time"

	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
	controlplanev1 "github.com/canonical/cluster-api-k8s/controlplane/api/v1beta2"
	"github.com/canonical/cluster-api-k8s/pkg/upgrade/inplace"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestIsUpgraded(t *testing.T) {
	g := NewWithT(t)

	for _, tc := range []struct {
		name          string
		annotations   map[string]string
		releaseString string
		isUpgraded    bool
	}{
		{
			name:          "notUpgraded",
			annotations:   map[string]string{bootstrapv1.InPlaceUpgradeReleaseAnnotation: "v1.30"},
			releaseString: "v1.29",
			isUpgraded:    false,
		},
		{
			name:          "noAnnotations",
			annotations:   map[string]string{},
			releaseString: "v1.29",
			isUpgraded:    false,
		},
		{
			name:          "upgraded",
			annotations:   map[string]string{bootstrapv1.InPlaceUpgradeReleaseAnnotation: "v1.30"},
			releaseString: "v1.30",
			isUpgraded:    true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			machine := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tc.annotations,
				},
			}

			result := inplace.IsUpgraded(machine, tc.releaseString)

			g.Expect(result).To(Equal(tc.isUpgraded))
		})
	}
}

func TestGetUpgradeInstructions(t *testing.T) {
	g := NewWithT(t)

	for _, tc := range []struct {
		name        string
		annotations map[string]string
		upgradeTo   string
		isUpgraded  bool
	}{
		{
			name: "InPlaceUpgradeReleaseAnnotationOnly",
			annotations: map[string]string{
				bootstrapv1.InPlaceUpgradeReleaseAnnotation: "v1.30",
			},
			upgradeTo: "v1.30",
		},
		{
			name: "InPlaceUpgradeToAnnotation",
			annotations: map[string]string{
				bootstrapv1.InPlaceUpgradeReleaseAnnotation: "v1.30",
				bootstrapv1.InPlaceUpgradeToAnnotation:      "v1.31",
			},
			upgradeTo: "v1.31",
		},
		{
			name:        "noAnnotations",
			annotations: map[string]string{},
			upgradeTo:   "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controlPlane := &controlplanev1.CK8sControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tc.annotations,
				},
			}

			result := inplace.GetUpgradeInstructions(controlPlane)

			g.Expect(result).To(Equal(tc.upgradeTo))
		})
	}
}

func TestIsMachineUpgradeFailed(t *testing.T) {
	g := NewWithT(t)
	for _, tc := range []struct {
		name                 string
		annotations          map[string]string
		machineUpgradeFailed bool
	}{
		{
			name:                 "InPlaceUpgradeLastFailedAttemptAtAnnotation",
			annotations:          map[string]string{bootstrapv1.InPlaceUpgradeLastFailedAttemptAtAnnotation: time.Now().Format(time.RFC1123Z)},
			machineUpgradeFailed: true,
		},
		{
			name:                 "noAnnotations",
			annotations:          map[string]string{},
			machineUpgradeFailed: false,
		},
		{
			name:                 "emptyInPlaceUpgradeLastFailedAttemptAtAnnotation",
			annotations:          map[string]string{bootstrapv1.InPlaceUpgradeLastFailedAttemptAtAnnotation: ""},
			machineUpgradeFailed: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			machine := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tc.annotations,
				},
			}

			result := inplace.IsMachineUpgradeFailed(machine)

			g.Expect(result).To(Equal(tc.machineUpgradeFailed))
		})
	}
}

func TestIsMachineUpgrading(t *testing.T) {
	g := NewWithT(t)
	for _, tc := range []struct {
		name             string
		annotations      map[string]string
		machineUpgrading bool
	}{
		{
			name: "InPlaceUpgradeStatusAnnotationInProgress",
			annotations: map[string]string{
				bootstrapv1.InPlaceUpgradeStatusAnnotation: bootstrapv1.InPlaceUpgradeInProgressStatus,
				bootstrapv1.InPlaceUpgradeToAnnotation:     "v1.31",
			},
			machineUpgrading: true,
		},
		{
			name: "InPlaceUpgradeStatusAnnotationDoneWithUpgradeTo",
			annotations: map[string]string{
				bootstrapv1.InPlaceUpgradeStatusAnnotation: bootstrapv1.InPlaceUpgradeDoneStatus,
				bootstrapv1.InPlaceUpgradeToAnnotation:     "v1.31",
			},
			machineUpgrading: true,
		},
		{
			name: "InPlaceUpgradeStatusAnnotationFailedWithUpgradeTo",
			annotations: map[string]string{
				bootstrapv1.InPlaceUpgradeStatusAnnotation: bootstrapv1.InPlaceUpgradeFailedStatus,
				bootstrapv1.InPlaceUpgradeToAnnotation:     "v1.31",
			},
			machineUpgrading: true,
		},
		{
			name: "InPlaceUpgradeStatusAnnotationDone",
			annotations: map[string]string{
				bootstrapv1.InPlaceUpgradeStatusAnnotation: bootstrapv1.InPlaceUpgradeDoneStatus,
			},
			machineUpgrading: false,
		},
		{
			name: "InPlaceUpgradeStatusAnnotationFailed",
			annotations: map[string]string{
				bootstrapv1.InPlaceUpgradeStatusAnnotation: bootstrapv1.InPlaceUpgradeFailedStatus,
			},
			machineUpgrading: false,
		},
		{
			name:             "noAnnotations",
			annotations:      map[string]string{},
			machineUpgrading: false,
		},
		{
			name: "NoInPlaceUpgradeToAnnotation",
			annotations: map[string]string{
				bootstrapv1.InPlaceUpgradeStatusAnnotation: bootstrapv1.InPlaceUpgradeFailedStatus,
			},
			machineUpgrading: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			machine := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tc.annotations,
				},
			}

			result := inplace.IsMachineUpgrading(machine)

			g.Expect(result).To(Equal(tc.machineUpgrading))
		})
	}
}
