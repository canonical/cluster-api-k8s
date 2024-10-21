package inplace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UpgradeLock is an interface that defines the methods used to lock and unlock the inplace upgrade process.
type UpgradeLock interface {
	// IsLocked checks if the upgrade process is locked and (if locked) returns the machine that the process is locked for.
	IsLocked(ctx context.Context, cluster *clusterv1.Cluster) (*clusterv1.Machine, error)
	// Lock is a non-blocking call that tries to lock the upgrade process for the given machine.
	Lock(ctx context.Context, cluster *clusterv1.Cluster, m *clusterv1.Machine) error
	// Unlock unlocks the upgrade process.
	Unlock(ctx context.Context, cluster *clusterv1.Cluster) error
}

const (
	lockConfigMapNameSuffix = "cp-inplace-upgrade-lock"
	lockInformationKey      = "lock-information"
)

func NewUpgradeLock(c client.Client) *upgradeLock {
	return &upgradeLock{
		c:         c,
		semaphore: &semaphore{},
	}
}

type upgradeLock struct {
	c         client.Client
	semaphore *semaphore
}

type semaphore struct {
	configMap *corev1.ConfigMap
}

func newSemaphore() *semaphore {
	return &semaphore{configMap: &corev1.ConfigMap{}}
}

func (s *semaphore) getLockInfo() (*lockInformation, error) {
	if s.configMap == nil {
		return nil, errors.New("configmap is nil")
	}
	if s.configMap.Data == nil {
		return nil, errors.New("configmap data is nil")
	}
	liStr, ok := s.configMap.Data[lockInformationKey]
	if !ok {
		return nil, errors.New("lock information key not found")
	}

	li := &lockInformation{}
	if err := json.Unmarshal([]byte(liStr), li); err != nil {
		return nil, fmt.Errorf("failed to unmarshal lock information: %w", err)
	}

	return li, nil
}

func (s *semaphore) setLockInfo(li lockInformation) error {
	if s.configMap == nil {
		s.configMap = &corev1.ConfigMap{}
	}
	if s.configMap.Data == nil {
		s.configMap.Data = make(map[string]string)
	}

	liStr, err := json.Marshal(li)
	if err != nil {
		return fmt.Errorf("failed to marshal lock information: %w", err)
	}

	s.configMap.Data[lockInformationKey] = string(liStr)
	return nil
}

func (s *semaphore) setMetadata(cluster *clusterv1.Cluster) {
	if s.configMap == nil {
		s.configMap = &corev1.ConfigMap{}
	}

	s.configMap.ObjectMeta = metav1.ObjectMeta{
		Namespace: cluster.Namespace,
		Name:      configMapName(cluster.Name),
		Labels: map[string]string{
			clusterv1.ClusterNameLabel: cluster.Name,
		},
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       clusterv1.ClusterKind,
				Name:       cluster.Name,
				UID:        cluster.UID,
			},
		},
	}
}

type lockInformation struct {
	MachineName      string `json:"machineName"`
	MachineNamespace string `json:"machineNamespace"`
}

func configMapName(clusterName string) string {
	return fmt.Sprintf("%s-%s", clusterName, lockConfigMapNameSuffix)
}

// IsLocked checks if the upgrade process is locked and (if locked) returns the machine that the process is locked for.
func (l *upgradeLock) IsLocked(ctx context.Context, cluster *clusterv1.Cluster) (*clusterv1.Machine, error) {
	l.semaphore = newSemaphore()
	name := configMapName(cluster.Name)
	if err := l.c.Get(ctx, client.ObjectKey{Name: name, Namespace: cluster.Namespace}, l.semaphore.configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get configmap %q: %w", name, err)
	}

	li, err := l.semaphore.getLockInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get lock information: %w", err)
	}

	machine := &clusterv1.Machine{}
	if err := l.c.Get(ctx, client.ObjectKey{Name: li.MachineName, Namespace: li.MachineNamespace}, machine); err != nil {
		// must be a stale lock from a deleted machine, unlock.
		if apierrors.IsNotFound(err) {
			if err := l.Unlock(ctx, cluster); err != nil {
				return nil, fmt.Errorf("failed to unlock: %w", err)
			}
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get machine %q: %w", li.MachineName, err)
	}

	return machine, nil
}

// Unlock unlocks the upgrade process.
func (l *upgradeLock) Unlock(ctx context.Context, cluster *clusterv1.Cluster) error {
	cm := &corev1.ConfigMap{}
	name := configMapName(cluster.Name)
	if err := l.c.Get(ctx, client.ObjectKey{Name: name, Namespace: cluster.Namespace}, cm); err != nil {
		// if the configmap is not found, it means the lock is already released.
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get configmap %q: %w", name, err)
	}

	if err := l.c.Delete(ctx, cm); err != nil {
		// if the configmap is not found, it means the lock is already released.
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete configmap %q: %w", name, err)
	}

	return nil
}

// Lock locks the upgrade process for the given machine.
func (l *upgradeLock) Lock(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	l.semaphore = newSemaphore()
	li := lockInformation{
		MachineName:      machine.Name,
		MachineNamespace: machine.Namespace,
	}
	if err := l.semaphore.setLockInfo(li); err != nil {
		return fmt.Errorf("failed to set lock information: %w", err)
	}
	l.semaphore.setMetadata(cluster)

	if err := l.c.Create(ctx, l.semaphore.configMap); err != nil {
		return fmt.Errorf("failed to create configmap: %w", err)
	}

	return nil
}

var _ UpgradeLock = &upgradeLock{}
