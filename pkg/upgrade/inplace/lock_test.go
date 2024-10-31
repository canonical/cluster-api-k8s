package inplace

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewUpgradeLock(t *testing.T) {
	g := NewWithT(t)
	testClient := fake.NewClientBuilder().Build()

	lock := NewUpgradeLock(testClient)

	g.Expect(lock).ToNot(BeNil())
	g.Expect(lock).To(BeAssignableToTypeOf(&upgradeLock{}))
}

func TestNewSemaphore(t *testing.T) {
	g := NewWithT(t)

	sem := newSemaphore()

	g.Expect(sem).ToNot(BeNil())
	g.Expect(sem).To(BeAssignableToTypeOf(&semaphore{}))
}

func TestSetLockInfo(t *testing.T) {
	g := NewWithT(t)
	machineName := "test-machine"
	machineNamespace := "test-namespace"
	sem := &semaphore{}
	li := lockInformation{
		MachineName:      machineName,
		MachineNamespace: machineNamespace,
	}

	err := sem.setLockInfo(li)

	g.Expect(err).ToNot(HaveOccurred())
	data, ok := sem.configMap.Data["lock-information"]
	g.Expect(ok).To(BeTrue())
	g.Expect(data).To(Equal(fmt.Sprintf("{\"machineName\":\"%s\",\"machineNamespace\":\"%s\"}",
		machineName, machineNamespace,
	)))
}
func TestGetLockInfo(t *testing.T) {
	g := NewWithT(t)
	t.Run("Success", func(t *testing.T) {
		machineName := "test-machine"
		machineNamespace := "test-namespace"
		lockConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster-cp-inplace-upgrade-lock",
				Namespace: "clusterNamespace",
			},
			Data: map[string]string{
				"lock-information": fmt.Sprintf("{\"machineName\":\"%s\",\"machineNamespace\":\"%s\"}",
					machineName, machineNamespace),
			},
		}
		sem := &semaphore{configMap: lockConfigMap}

		lockInfo, err := sem.getLockInfo()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(lockInfo.MachineName).To(Equal(machineName))
		g.Expect(lockInfo.MachineNamespace).To(Equal(machineNamespace))
	})

	t.Run("configMapIsNil", func(t *testing.T) {
		sem := &semaphore{configMap: nil}

		lockInfo, err := sem.getLockInfo()

		g.Expect(err).To(HaveOccurred())
		g.Expect(lockInfo).To(BeNil())
	})

	t.Run("configMapDataIsNil", func(t *testing.T) {
		lockConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster-cp-inplace-upgrade-lock",
				Namespace: "clusterNamespace",
			},
		}
		sem := &semaphore{configMap: lockConfigMap}

		lockInfo, err := sem.getLockInfo()

		g.Expect(err).To(HaveOccurred())
		g.Expect(lockInfo).To(BeNil())
	})
}

func TestSetMetadata(t *testing.T) {
	g := NewWithT(t)

	clusterName := "test-cluster"
	clusterNamespace := "test-namespace"
	clusterUID := types.UID("50f4a6af-39be-4589-abf0-0a71110fda00")
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: clusterNamespace,
			UID:       clusterUID,
		},
	}
	sem := &semaphore{}

	sem.setMetadata(cluster)

	meta := sem.configMap.ObjectMeta
	g.Expect(meta.Namespace).To(Equal(clusterNamespace))
	g.Expect(meta.Name).To(Equal(fmt.Sprintf("%s-%s", clusterName, lockConfigMapNameSuffix)))
	g.Expect(meta.Labels[clusterv1.ClusterNameLabel]).To(Equal(clusterName))
	g.Expect(meta.OwnerReferences[0].Name).To(Equal(clusterName))
	g.Expect(meta.OwnerReferences[0].UID).To(Equal(clusterUID))
}

func TestIsLocked(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		g := NewWithT(t)

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-namespace",
			},
		}
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machine",
				Namespace: "test-namespace",
			},
		}
		li := lockInformation{
			MachineName:      machine.Name,
			MachineNamespace: machine.Namespace,
		}
		liStr, err := json.Marshal(li)
		g.Expect(err).ToNot(HaveOccurred())
		lockConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-cp-inplace-upgrade-lock", cluster.Name),
				Namespace: cluster.Namespace,
			},
			Data: map[string]string{
				"lock-information": string(liStr),
			},
		}
		scheme := runtime.NewScheme()
		err = clusterv1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		err = corev1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		testClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				cluster,
				machine,
				lockConfigMap,
			).
			Build()
		lock := NewUpgradeLock(testClient)
		err = lock.semaphore.setLockInfo(li)
		ctx := context.Background()
		lock.semaphore.setMetadata(cluster)
		g.Expect(err).ToNot(HaveOccurred())

		machine2, err := lock.IsLocked(ctx, cluster)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(machine2).ToNot(BeNil())
	})

	t.Run("NoConfigMap", func(t *testing.T) {
		g := NewWithT(t)
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-namespace",
			},
		}
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machine",
				Namespace: "test-namespace",
			},
		}
		li := lockInformation{
			MachineName:      machine.Name,
			MachineNamespace: machine.Namespace,
		}
		scheme := runtime.NewScheme()
		err := clusterv1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		err = corev1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		testClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				cluster,
				machine,
			).
			Build()
		lock := NewUpgradeLock(testClient)
		err = lock.semaphore.setLockInfo(li)
		ctx := context.Background()
		lock.semaphore.setMetadata(cluster)
		g.Expect(err).ToNot(HaveOccurred())

		machine2, err := lock.IsLocked(ctx, cluster)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(machine2).To(BeNil())
	})

	t.Run("noMachine", func(t *testing.T) {
		g := NewWithT(t)
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-namespace",
			},
		}
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machine",
				Namespace: "test-namespace",
			},
		}
		li := lockInformation{
			MachineName:      machine.Name,
			MachineNamespace: machine.Namespace,
		}
		liStr, err := json.Marshal(li)
		g.Expect(err).ToNot(HaveOccurred())
		lockConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-cp-inplace-upgrade-lock", cluster.Name),
				Namespace: cluster.Namespace,
			},
			Data: map[string]string{
				"lock-information": string(liStr),
			},
		}
		scheme := runtime.NewScheme()
		err = clusterv1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		err = corev1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		testClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				cluster,
				lockConfigMap,
			).
			Build()
		lock := NewUpgradeLock(testClient)
		err = lock.semaphore.setLockInfo(li)
		ctx := context.Background()
		lock.semaphore.setMetadata(cluster)
		g.Expect(err).ToNot(HaveOccurred())

		machine2, err := lock.IsLocked(ctx, cluster)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(machine2).To(BeNil())
	})

	t.Run("NoLockData", func(t *testing.T) {
		g := NewWithT(t)
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-namespace",
			},
		}
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machine",
				Namespace: "test-namespace",
			},
		}
		li := lockInformation{
			MachineName:      machine.Name,
			MachineNamespace: machine.Namespace,
		}
		lockConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-cp-inplace-upgrade-lock", cluster.Name),
				Namespace: cluster.Namespace,
			},
		}
		scheme := runtime.NewScheme()
		err := clusterv1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		err = corev1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		testClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				cluster,
				machine,
				lockConfigMap,
			).
			Build()
		lock := NewUpgradeLock(testClient)
		err = lock.semaphore.setLockInfo(li)
		ctx := context.Background()
		lock.semaphore.setMetadata(cluster)
		g.Expect(err).ToNot(HaveOccurred())

		machine2, err := lock.IsLocked(ctx, cluster)

		g.Expect(err).To(HaveOccurred())
		g.Expect(machine2).To(BeNil())
	})
}

func TestLock(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		g := NewWithT(t)
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-namespace",
			},
		}
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machine",
				Namespace: "test-namespace",
			},
		}
		scheme := runtime.NewScheme()
		err := clusterv1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		err = corev1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		testClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				cluster,
				machine,
			).
			Build()
		lock := NewUpgradeLock(testClient)

		err = lock.Lock(context.Background(), cluster, machine)

		g.Expect(err).ToNot(HaveOccurred())
		lockInfo, err := lock.semaphore.getLockInfo()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(lockInfo.MachineName).To(Equal(machine.Name))
		g.Expect(lockInfo.MachineNamespace).To(Equal(machine.Namespace))
	})
	t.Run("LockAlreadyExists", func(t *testing.T) {
		g := NewWithT(t)
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-namespace",
			},
		}
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machine",
				Namespace: "test-namespace",
			},
		}
		existingLockConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-cp-inplace-upgrade-lock", cluster.Name),
				Namespace: cluster.Namespace,
			},
		}
		scheme := runtime.NewScheme()
		err := clusterv1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		err = corev1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		testClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				cluster,
				machine,
				existingLockConfigMap,
			).
			Build()
		lock := NewUpgradeLock(testClient)

		err = lock.Lock(context.Background(), cluster, machine)

		g.Expect(err).To(HaveOccurred())
		lockInfo, err := lock.semaphore.getLockInfo()
		g.Expect(err).To(HaveOccurred())
		g.Expect(lockInfo.MachineName).To(Equal(machine.Name))
		g.Expect(lockInfo.MachineNamespace).To(Equal(machine.Namespace))
	})
}

func TestUnlock(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		g := NewWithT(t)
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-namespace",
			},
		}
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machine",
				Namespace: "test-namespace",
			},
		}
		li := lockInformation{
			MachineName:      machine.Name,
			MachineNamespace: machine.Namespace,
		}
		liStr, err := json.Marshal(li)
		g.Expect(err).ToNot(HaveOccurred())
		lockConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-cp-inplace-upgrade-lock", cluster.Name),
				Namespace: cluster.Namespace,
			},
			Data: map[string]string{
				"lock-information": string(liStr),
			},
		}
		scheme := runtime.NewScheme()
		err = clusterv1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		err = corev1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		testClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				cluster,
				machine,
				lockConfigMap,
			).
			Build()
		lock := NewUpgradeLock(testClient)

		err = lock.Unlock(context.Background(), cluster)

		g.Expect(err).ToNot(HaveOccurred())
		err = testClient.Get(context.Background(),
			client.ObjectKey{
				Name:      fmt.Sprintf("%s-cp-inplace-upgrade-lock", cluster.Name),
				Namespace: cluster.Namespace,
			},
			&corev1.ConfigMap{})
		g.Expect(err).NotTo(BeNil())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
	t.Run("LockAlreadyReleased", func(t *testing.T) {
		g := NewWithT(t)
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-namespace",
			},
		}
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machine",
				Namespace: "test-namespace",
			},
		}
		scheme := runtime.NewScheme()
		err := clusterv1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		err = corev1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())
		testClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				cluster,
				machine,
			).
			Build()
		lock := NewUpgradeLock(testClient)

		err = lock.Unlock(context.Background(), cluster)

		g.Expect(err).ToNot(HaveOccurred())
		err = testClient.Get(context.Background(), client.ObjectKey{Name: fmt.Sprintf("%s-cp-inplace-upgrade-lock", cluster.Name), Namespace: cluster.Namespace}, &corev1.ConfigMap{})
		g.Expect(err).NotTo(BeNil())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
}
