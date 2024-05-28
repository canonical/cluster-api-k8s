package machinefilters

import (
	"testing"

	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
	controlplanev1 "github.com/canonical/cluster-api-k8s/controlplane/api/v1beta2"
)

func TestMatchesCK8sBootstrapConfig(t *testing.T) {
	t.Run("returns true if ClusterConfiguration is equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.CK8sControlPlane{
			Spec: controlplanev1.CK8sControlPlaneSpec{
				CK8sConfigSpec: bootstrapv1.CK8sConfigSpec{
					ServerConfig: bootstrapv1.CK8sServerConfig{
						ClusterDomain: "foo",
					},
				},
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.CK8sServerConfigurationAnnotation: "{\n  \"clusterDomain\": \"foo\"\n}",
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.CK8sConfig{
			m.Name: {},
		}
		match := MatchesCK8sBootstrapConfig(machineConfigs, kcp)(m)
		g.Expect(match).To(BeTrue())
	})
	t.Run("returns false if ClusterConfiguration is NOT equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.CK8sControlPlane{
			Spec: controlplanev1.CK8sControlPlaneSpec{
				CK8sConfigSpec: bootstrapv1.CK8sConfigSpec{
					ServerConfig: bootstrapv1.CK8sServerConfig{
						ClusterDomain: "foo",
					},
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "CK8sConfig",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "CK8sConfig",
						Namespace:  "default",
						Name:       "test",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.CK8sConfig{
			m.Name: {
				TypeMeta: metav1.TypeMeta{
					Kind:       "CK8sConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.CK8sConfigSpec{
					ServerConfig: bootstrapv1.CK8sServerConfig{
						ClusterDomain: "bar",
					},
				},
			},
		}
		match := MatchesCK8sBootstrapConfig(machineConfigs, kcp)(m)
		g.Expect(match).To(BeFalse())
	})

	t.Run("returns false if some other configurations are not equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.CK8sControlPlane{
			Spec: controlplanev1.CK8sControlPlaneSpec{
				CK8sConfigSpec: bootstrapv1.CK8sConfigSpec{
					Files: []bootstrapv1.File{}, // This is a change
				},
			},
		}

		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "CK8sConfig",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "CK8sConfig",
						Namespace:  "default",
						Name:       "test",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.CK8sConfig{
			m.Name: {
				TypeMeta: metav1.TypeMeta{
					Kind:       "CK8sConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.CK8sConfigSpec{},
			},
		}
		match := MatchesCK8sBootstrapConfig(machineConfigs, kcp)(m)
		g.Expect(match).To(BeFalse())
	})

	t.Run("Should match on other configurations", func(t *testing.T) {
		kThreesConfigSpec := bootstrapv1.CK8sConfigSpec{
			Files:           []bootstrapv1.File{},
			PreK3sCommands:  []string{"test"},
			PostK3sCommands: []string{"test"},
			AgentConfig: bootstrapv1.CK8sAgentConfig{
				NodeName:      "test-node",
				NodeTaints:    []string{"node-role.kubernetes.io/control-plane:NoSchedule"},
				KubeProxyArgs: []string{"metrics-bind-address=0.0.0.0"},
			},
			ServerConfig: bootstrapv1.CK8sServerConfig{
				DisableComponents:         []string{"traefik"},
				KubeControllerManagerArgs: []string{"bind-address=0.0.0.0"},
				KubeSchedulerArgs:         []string{"bind-address=0.0.0.0"},
			},
		}
		kcp := &controlplanev1.CK8sControlPlane{
			Spec: controlplanev1.CK8sControlPlaneSpec{
				Replicas: proto.Int32(3),
				Version:  "v1.13.14+k3s1",
				MachineTemplate: controlplanev1.CK8sControlPlaneMachineTemplate{
					ObjectMeta: clusterv1.ObjectMeta{
						Labels: map[string]string{"test-label": "test-value"},
					},
				},
				CK8sConfigSpec: kThreesConfigSpec,
			},
		}

		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "CK8sConfig",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "CK8sConfig",
						Namespace:  "default",
						Name:       "test",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.CK8sConfig{
			m.Name: {
				TypeMeta: metav1.TypeMeta{
					Kind:       "CK8sConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: kThreesConfigSpec,
			},
		}

		t.Run("by returning true if all configs match", func(t *testing.T) {
			g := NewWithT(t)
			match := MatchesCK8sBootstrapConfig(machineConfigs, kcp)(m)
			g.Expect(match).To(BeTrue())
		})

		t.Run("by returning false if post commands don't match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Spec.PostK3sCommands = []string{"new-test"}
			match := MatchesCK8sBootstrapConfig(machineConfigs, kcp)(m)
			g.Expect(match).To(BeFalse())
		})

		t.Run("by returning false if agent configs don't match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Spec.AgentConfig.KubeletArgs = []string{"test-arg"}
			match := MatchesCK8sBootstrapConfig(machineConfigs, kcp)(m)
			g.Expect(match).To(BeFalse())
		})
	})

	t.Run("should match on labels and annotations", func(t *testing.T) {
		kcp := &controlplanev1.CK8sControlPlane{
			Spec: controlplanev1.CK8sControlPlaneSpec{
				MachineTemplate: controlplanev1.CK8sControlPlaneMachineTemplate{
					ObjectMeta: clusterv1.ObjectMeta{
						Annotations: map[string]string{
							"test": "annotation",
						},
						Labels: map[string]string{
							"test": "labels",
						},
					},
				},
				CK8sConfigSpec: bootstrapv1.CK8sConfigSpec{
					ServerConfig: bootstrapv1.CK8sServerConfig{},
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "CK8sConfig",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "CK8sConfig",
						Namespace:  "default",
						Name:       "test",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.CK8sConfig{
			m.Name: {
				TypeMeta: metav1.TypeMeta{
					Kind:       "CK8sConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.CK8sConfigSpec{
					ServerConfig: bootstrapv1.CK8sServerConfig{},
				},
			},
		}

		t.Run("by returning true if neither labels or annotations match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Annotations = nil
			machineConfigs[m.Name].Labels = nil
			match := MatchesCK8sBootstrapConfig(machineConfigs, kcp)(m)
			g.Expect(match).To(BeTrue())
		})

		t.Run("by returning true if only labels don't match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Annotations = kcp.Spec.MachineTemplate.ObjectMeta.Annotations
			machineConfigs[m.Name].Labels = nil
			match := MatchesCK8sBootstrapConfig(machineConfigs, kcp)(m)
			g.Expect(match).To(BeTrue())
		})

		t.Run("by returning true if only annotations don't match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Annotations = nil
			machineConfigs[m.Name].Labels = kcp.Spec.MachineTemplate.ObjectMeta.Labels
			match := MatchesCK8sBootstrapConfig(machineConfigs, kcp)(m)
			g.Expect(match).To(BeTrue())
		})

		t.Run("by returning true if both labels and annotations match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Labels = kcp.Spec.MachineTemplate.ObjectMeta.Labels
			machineConfigs[m.Name].Annotations = kcp.Spec.MachineTemplate.ObjectMeta.Annotations
			match := MatchesCK8sBootstrapConfig(machineConfigs, kcp)(m)
			g.Expect(match).To(BeTrue())
		})
	})
}
