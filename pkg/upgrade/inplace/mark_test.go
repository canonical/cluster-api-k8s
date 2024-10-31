package inplace_test

import (
	"context"
	"testing"

	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
	controlplanev1 "github.com/canonical/cluster-api-k8s/controlplane/api/v1beta2"
	"github.com/canonical/cluster-api-k8s/pkg/upgrade/inplace"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMarkMachineToUpgrade(t *testing.T) {
	g := NewWithT(t)

	for _, tc := range []struct {
		name               string
		annotations        map[string]string
		ToAnnotation       string
		addMachineToClient bool
	}{
		{
			name: "notUpgraded",
			annotations: map[string]string{
				bootstrapv1.InPlaceUpgradeReleaseAnnotation:             "v1.30",
				bootstrapv1.InPlaceUpgradeStatusAnnotation:              "failed",
				bootstrapv1.InPlaceUpgradeChangeIDAnnotation:            "123",
				bootstrapv1.InPlaceUpgradeLastFailedAttemptAtAnnotation: "Wed, 30 Oct 2024 09:47:37 +0100",
			},
			ToAnnotation:       "v1.29",
			addMachineToClient: true,
		},
		{
			name:               "noAnnotations",
			annotations:        nil,
			ToAnnotation:       "v1.29",
			addMachineToClient: true,
		},
		{
			name: "FailedToPatch",
			annotations: map[string]string{
				bootstrapv1.InPlaceUpgradeReleaseAnnotation:             "v1.30",
				bootstrapv1.InPlaceUpgradeStatusAnnotation:              "failed",
				bootstrapv1.InPlaceUpgradeChangeIDAnnotation:            "123",
				bootstrapv1.InPlaceUpgradeLastFailedAttemptAtAnnotation: "Wed, 30 Oct 2024 09:47:37 +0100",
			},
			ToAnnotation:       "v1.29",
			addMachineToClient: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			machine := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test",
					Annotations: tc.annotations,
				},
			}
			scheme := runtime.NewScheme()
			err := clusterv1.AddToScheme(scheme)
			g.Expect(err).ToNot(HaveOccurred())

			testClientBuilder := fake.NewClientBuilder().
				WithScheme(scheme)
			if tc.addMachineToClient {
				testClientBuilder = testClientBuilder.WithObjects(machine.DeepCopy())
			}
			testClient := testClientBuilder.Build()

			res := inplace.MarkMachineToUpgrade(context.Background(), machine, tc.ToAnnotation, testClient)
			if tc.addMachineToClient {
				g.Expect(res).ToNot(HaveOccurred())
			} else {
				g.Expect(res).To(HaveOccurred())
			}
			g.Expect(machine.ObjectMeta.Annotations).ShouldNot(ContainElements(
				bootstrapv1.InPlaceUpgradeReleaseAnnotation,
				bootstrapv1.InPlaceUpgradeStatusAnnotation,
				bootstrapv1.InPlaceUpgradeChangeIDAnnotation,
				bootstrapv1.InPlaceUpgradeLastFailedAttemptAtAnnotation,
			))
			g.Expect(machine.ObjectMeta.Annotations[bootstrapv1.InPlaceUpgradeToAnnotation]).Should(Equal(tc.ToAnnotation))
		})
	}

	t.Run("nilMachine", func(t *testing.T) {
		scheme := runtime.NewScheme()
		err := clusterv1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		testClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		res := inplace.MarkMachineToUpgrade(context.Background(), nil, "", testClient)

		g.Expect(res).To(HaveOccurred())
	})
}

func TestMarkUpgradeFailed(t *testing.T) {
	g := NewWithT(t)

	for _, tc := range []struct {
		name        string
		annotations map[string]string
	}{
		{
			name: "InPlaceUpgradeInProgressStatus",
			annotations: map[string]string{
				bootstrapv1.InPlaceUpgradeStatusAnnotation: bootstrapv1.InPlaceUpgradeInProgressStatus,
			},
		},
		{
			name:        "NoAnnotations",
			annotations: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controlPlane := &controlplanev1.CK8sControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "cp_test",
					Annotations: tc.annotations,
				},
			}
			scheme := runtime.NewScheme()
			err := controlplanev1.AddToScheme(scheme)
			g.Expect(err).ToNot(HaveOccurred())

			testClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(controlPlane.DeepCopy()).
				Build()
			helper, err := patch.NewHelper(controlPlane, testClient)
			g.Expect(err).ToNot(HaveOccurred())

			result := inplace.MarkUpgradeFailed(context.Background(), controlPlane, helper)

			g.Expect(result).NotTo(HaveOccurred())
			g.Expect(controlPlane.ObjectMeta.Annotations[bootstrapv1.InPlaceUpgradeStatusAnnotation]).To(Equal(bootstrapv1.InPlaceUpgradeFailedStatus))
		})
	}
	t.Run("ControlPlaneNotRegistered", func(t *testing.T) {
		controlPlane := &controlplanev1.CK8sControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cp_test",
			},
		}
		scheme := runtime.NewScheme()
		err := controlplanev1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		testClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()
		helper, err := patch.NewHelper(controlPlane, testClient)
		g.Expect(err).ToNot(HaveOccurred())

		result := inplace.MarkUpgradeFailed(context.Background(), controlPlane, helper)

		g.Expect(result).To(HaveOccurred())
	})
}

func TestMarkUpgradeProgress(t *testing.T) {
	g := NewWithT(t)
	for _, tc := range []struct {
		name        string
		annotations map[string]string
		to          string
	}{
		{
			name: "InPlaceUpgradeInProgressStatus",
			annotations: map[string]string{
				bootstrapv1.InPlaceUpgradeToAnnotation: "v1.29",
			},
			to: "v1.30",
		},
		{
			name:        "NoAnnotations",
			annotations: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controlPlane := &controlplanev1.CK8sControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "cp_test",
					Annotations: tc.annotations,
				},
			}
			scheme := runtime.NewScheme()
			err := controlplanev1.AddToScheme(scheme)
			g.Expect(err).ToNot(HaveOccurred())
			testClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(controlPlane.DeepCopy()).
				Build()
			helper, err := patch.NewHelper(controlPlane, testClient)
			g.Expect(err).ToNot(HaveOccurred())

			result := inplace.MarkUpgradeInProgress(context.Background(), controlPlane, tc.to, helper)

			g.Expect(result).NotTo(HaveOccurred())
			g.Expect(controlPlane.ObjectMeta.Annotations[bootstrapv1.InPlaceUpgradeStatusAnnotation]).To(Equal(bootstrapv1.InPlaceUpgradeInProgressStatus))
			g.Expect(controlPlane.ObjectMeta.Annotations[bootstrapv1.InPlaceUpgradeToAnnotation]).To(Equal(tc.to))
		})
	}
	t.Run("ControlPlaneNotRegistered", func(t *testing.T) {
		controlPlane := &controlplanev1.CK8sControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cp_test",
			},
		}
		scheme := runtime.NewScheme()
		err := controlplanev1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		testClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()
		helper, err := patch.NewHelper(controlPlane, testClient)
		g.Expect(err).ToNot(HaveOccurred())

		result := inplace.MarkUpgradeInProgress(context.Background(), controlPlane, "v1.29", helper)

		g.Expect(result).To(HaveOccurred())
	})
}

func TestMarkUpgradeDone(t *testing.T) {
	g := NewWithT(t)
	for _, tc := range []struct {
		name        string
		annotations map[string]string
		to          string
	}{
		{
			name: "InPlaceUpgradeInProgressStatus",
			annotations: map[string]string{
				bootstrapv1.InPlaceUpgradeToAnnotation: "v1.29",
			},
			to: "v1.30",
		},
		{
			name:        "NoAnnotations",
			annotations: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controlPlane := &controlplanev1.CK8sControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "cp_test",
					Annotations: tc.annotations,
				},
			}
			scheme := runtime.NewScheme()
			err := controlplanev1.AddToScheme(scheme)
			g.Expect(err).ToNot(HaveOccurred())
			testClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(controlPlane.DeepCopy()).
				Build()
			helper, err := patch.NewHelper(controlPlane, testClient)
			g.Expect(err).ToNot(HaveOccurred())

			result := inplace.MarkUpgradeDone(context.Background(), controlPlane, tc.to, helper)

			g.Expect(result).NotTo(HaveOccurred())
			g.Expect(controlPlane.ObjectMeta.Annotations[bootstrapv1.InPlaceUpgradeStatusAnnotation]).To(Equal(bootstrapv1.InPlaceUpgradeDoneStatus))
			g.Expect(controlPlane.ObjectMeta.Annotations[bootstrapv1.InPlaceUpgradeReleaseAnnotation]).To(Equal(tc.to))
		})
	}
	t.Run("ControlPlaneNotRegistered", func(t *testing.T) {
		controlPlane := &controlplanev1.CK8sControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cp_test",
			},
		}
		scheme := runtime.NewScheme()
		err := controlplanev1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		testClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()
		helper, err := patch.NewHelper(controlPlane, testClient)
		g.Expect(err).ToNot(HaveOccurred())

		result := inplace.MarkUpgradeInProgress(context.Background(), controlPlane, "v1.29", helper)

		g.Expect(result).To(HaveOccurred())
	})
}
