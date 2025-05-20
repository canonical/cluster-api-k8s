package ck8s

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestClusterStatus(t *testing.T) {
	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1",
			Labels: map[string]string{
				labelNodeRoleControlPlane: "",
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}
	node2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node2",
			Labels: map[string]string{
				labelNodeRoleControlPlane: "",
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionFalse,
			}},
		},
	}
	servingSecret := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sdConfigSecretName,
			Namespace: metav1.NamespaceSystem,
		},
	}
	tests := []struct {
		name            string
		objs            []client.Object
		expectErr       bool
		expectHasSecret bool
	}{
		{
			name:            "returns cluster status",
			objs:            []client.Object{},
			expectHasSecret: false,
			expectErr:       true,
		},
		{
			name:            "returns cluster status with k8sd-config configmap",
			objs:            []client.Object{servingSecret, node1, node2},
			expectHasSecret: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(tt.objs...).Build()
			w := &Workload{
				Client: fakeClient,
			}
			status, err := w.ClusterStatus(context.TODO())
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(status.Nodes).To(BeEquivalentTo(0))
				g.Expect(status.ReadyNodes).To(BeEquivalentTo(0))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(status.Nodes).To(BeEquivalentTo(2))
				g.Expect(status.ReadyNodes).To(BeEquivalentTo(1))
			}
			g.Expect(status.HasK8sdConfigMap).To(Equal(tt.expectHasSecret))
		})
	}
}
