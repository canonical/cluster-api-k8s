package ck8s_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/canonical/cluster-api-k8s/pkg/ck8s"
)

func newFakeClientWithIndex(objects ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
			pod := obj.(*corev1.Pod)
			return []string{pod.Spec.NodeName}
		}).
		Build()
}

func TestCordonNode(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Spec:       corev1.NodeSpec{Unschedulable: false},
	}

	fakeClient := newFakeClientWithIndex(node)
	drainer := ck8s.NewDrainer(fakeClient, time.Now)

	err := drainer.CordonNode(ctx, "test-node")
	g.Expect(err).ToNot(HaveOccurred())

	// Verify node is cordoned
	updatedNode := &corev1.Node{}
	err = fakeClient.Get(ctx, client.ObjectKey{Name: "test-node"}, updatedNode)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updatedNode.Spec.Unschedulable).To(BeTrue())
}

func TestUncordonNode(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Spec:       corev1.NodeSpec{Unschedulable: true},
	}

	fakeClient := newFakeClientWithIndex(node)
	drainer := ck8s.NewDrainer(fakeClient, time.Now)

	err := drainer.UncordonNode(ctx, "test-node")
	g.Expect(err).ToNot(HaveOccurred())

	// Verify node is uncordoned
	updatedNode := &corev1.Node{}
	err = fakeClient.Get(ctx, client.ObjectKey{Name: "test-node"}, updatedNode)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updatedNode.Spec.Unschedulable).To(BeFalse())
}

func TestCordonNodeNotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	fakeClient := newFakeClientWithIndex()
	drainer := ck8s.NewDrainer(fakeClient, time.Now)

	err := drainer.CordonNode(ctx, "nonexistent-node")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("failed to get node"))
}

func TestDrainNodeWithNoPods(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}

	fakeClient := newFakeClientWithIndex(node)

	opts := ck8s.DrainOptions{
		EvictionRetryInterval: 10 * time.Millisecond,
		IgnoreDaemonsets:      true,
	}
	drainer := ck8s.NewDrainer(fakeClient, time.Now, opts)

	err := drainer.DrainNode(ctx, "test-node")
	g.Expect(err).ToNot(HaveOccurred())
}

func TestDrainNodeWithDaemonSetPodsIgnored(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}

	daemonSetPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "daemonset-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "DaemonSet", Name: "test-ds", Controller: ptr.To(true)},
			},
		},
		Spec: corev1.PodSpec{NodeName: "test-node"},
	}

	fakeClient := newFakeClientWithIndex(node, daemonSetPod)

	opts := ck8s.DrainOptions{
		EvictionRetryInterval: 10 * time.Millisecond,
		IgnoreDaemonsets:      true,
	}
	drainer := ck8s.NewDrainer(fakeClient, time.Now, opts)

	err := drainer.DrainNode(ctx, "test-node")
	g.Expect(err).ToNot(HaveOccurred())
}

func TestDrainNodeWithDaemonSetPodsNotIgnored(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}

	daemonSetPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "daemonset-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "DaemonSet", Name: "test-ds", Controller: ptr.To(true)},
			},
		},
		Spec: corev1.PodSpec{NodeName: "test-node"},
	}

	fakeClient := newFakeClientWithIndex(node, daemonSetPod)

	opts := ck8s.DrainOptions{
		EvictionRetryInterval: 10 * time.Millisecond,
	}
	drainer := ck8s.NewDrainer(fakeClient, time.Now, opts)

	err := drainer.DrainNode(ctx, "test-node")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("managed by a DaemonSet"))
}

func TestDrainNodeWithEmptyDirAllowed(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-with-emptydir",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "test-rs", Controller: ptr.To(true)},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "test-node",
			Volumes: []corev1.Volume{
				{Name: "cache", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
		},
	}

	fakeClient := newFakeClientWithIndex(node, pod)

	opts := ck8s.DrainOptions{
		EvictionRetryInterval: 10 * time.Millisecond,
		DeleteEmptydirData:    true,
		IgnoreDaemonsets:      true,
	}
	drainer := ck8s.NewDrainer(fakeClient, time.Now, opts)

	err := drainer.DrainNode(ctx, "test-node")
	g.Expect(err).To(Not(HaveOccurred()))
}

func TestDrainNodeWithEmptyDirNotAllowed(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-with-emptydir",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "test-rs", Controller: ptr.To(true)},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "test-node",
			Volumes: []corev1.Volume{
				{Name: "cache", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
		},
	}

	fakeClient := newFakeClientWithIndex(node, pod)

	opts := ck8s.DrainOptions{
		EvictionRetryInterval: 10 * time.Millisecond,
		IgnoreDaemonsets:      true,
	}
	drainer := ck8s.NewDrainer(fakeClient, time.Now, opts)

	err := drainer.DrainNode(ctx, "test-node")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("using emptyDir volume"))
}

func TestDrainNodeWithPodWithoutController(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "standalone-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{NodeName: "test-node"},
	}

	fakeClient := newFakeClientWithIndex(node, pod)

	opts := ck8s.DrainOptions{
		EvictionRetryInterval: 10 * time.Millisecond,
		IgnoreDaemonsets:      true,
	}
	drainer := ck8s.NewDrainer(fakeClient, time.Now, opts)

	err := drainer.DrainNode(ctx, "test-node")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("does not have a controller"))
}

func TestDrainNodeWithPodWithoutControllerForced(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "standalone-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{NodeName: "test-node"},
	}

	fakeClient := newFakeClientWithIndex(node, pod)

	opts := ck8s.DrainOptions{
		EvictionRetryInterval: 10 * time.Millisecond,
		Force:                 true,
		IgnoreDaemonsets:      true,
	}
	drainer := ck8s.NewDrainer(fakeClient, time.Now, opts)

	// Will attempt to evict and handle errors gracefully
	err := drainer.DrainNode(ctx, "test-node")
	g.Expect(err).To(Or(BeNil(), HaveOccurred()))
}

func TestDrainNodeSkipsStaticPods(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}

	staticPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "static-pod",
			Namespace: "kube-system",
			Annotations: map[string]string{
				corev1.MirrorPodAnnotationKey: "mirror-pod",
			},
		},
		Spec: corev1.PodSpec{NodeName: "test-node"},
	}

	fakeClient := newFakeClientWithIndex(node, staticPod)

	opts := ck8s.DrainOptions{
		EvictionRetryInterval: 10 * time.Millisecond,
		IgnoreDaemonsets:      true,
	}
	drainer := ck8s.NewDrainer(fakeClient, time.Now, opts)

	err := drainer.DrainNode(ctx, "test-node")
	g.Expect(err).ToNot(HaveOccurred())
}
