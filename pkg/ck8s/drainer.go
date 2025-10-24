package ck8s

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Drainer defines the interface for draining and cordoning nodes.
type Drainer interface {
	// DrainNode drains the specified node by evicting its pods according to the drain options.
	DrainNode(ctx context.Context, nodeName string) error
	// CordonNode marks the specified node as unschedulable.
	CordonNode(ctx context.Context, nodeName string) error
	// UncordonNode marks the specified node as schedulable.
	UncordonNode(ctx context.Context, nodeName string) error
}

// DrainOptions defines options for draining a node.
type DrainOptions struct {
	// Timeout is the maximum duration to wait for the drain operation to complete.
	Timeout time.Duration
	// DeleteEmptydirData indicates whether to delete pods using emptyDir volumes.
	// Local data that will be deleted when the node is drained.
	// Equivalent to --delete-emptydir-data flag in kubectl drain.
	DeleteEmptydirData bool
	// Force indicates whether to Force drain even if there are pods without controllers.
	// Equivalent to --Force flag in kubectl drain.
	Force bool
	// GracePeriodSeconds period of time in seconds given to each pod to terminate gracefully.
	// If negative, the default value specified in the pod will be used.
	// Equivalent to --grace-period flag in kubectl drain.
	GracePeriodSeconds int64
	// IgnoreDaemonsets indicates whether to ignore DaemonSet-managed pods.
	// Equivalent to --ignore-daemonsets flag in kubectl drain.
	IgnoreDaemonsets bool
	// AllowDeletion indicates whether to allow deletion of pods that are blocked by PodDisruptionBudgets.
	// If true, pods that cannot be evicted due to PDB constraints will be force deleted.
	AllowDeletion bool
	// EvictionRetryInterval is the duration to wait between retries when evicting or deleting pods.
	EvictionRetryInterval time.Duration
	// EvictionTimeout is the maximum duration to wait for the a single cycle of eviction or deletion to complete.
	// Note that DrainNode may perform multiple cycles to evict all pods.
	EvictionTimeout time.Duration
}

func (o DrainOptions) defaults() DrainOptions {
	return DrainOptions{
		GracePeriodSeconds: -1,
	}
}

type drainer struct {
	client ctrlclient.Client
	// nowFunc is a function that returns the current time.
	// It is used to facilitate testing.
	nowFunc func() time.Time
	opts    DrainOptions
}

func NewDrainer(client ctrlclient.Client, nowFunc func() time.Time, opts ...DrainOptions) *drainer {
	o := DrainOptions{}.defaults()
	if len(opts) > 0 {
		o = opts[0]
	}

	return &drainer{
		client:  client,
		nowFunc: nowFunc,
		opts:    o,
	}
}

// CordonNode marks the specified node as unschedulable.
func (d *drainer) CordonNode(ctx context.Context, nodeName string) error {
	log := log.FromContext(ctx).WithValues("node", nodeName, "scope", "CordonNode")

	node := &corev1.Node{}
	if err := d.client.Get(ctx, ctrlclient.ObjectKey{Name: nodeName}, node); err != nil {
		return fmt.Errorf("failed to get node: %w", err)
	}

	patch := ctrlclient.StrategicMergeFrom(node, ctrlclient.MergeFromWithOptimisticLock{})

	newNode := node.DeepCopy()
	newNode.Spec.Unschedulable = true

	if err := d.client.Patch(ctx, newNode, patch); err != nil {
		return fmt.Errorf("failed to patch node: %w", err)
	}

	log.Info("Node cordoned successfully")
	return nil
}

// UncordonNode marks the specified node as schedulable.
func (d *drainer) UncordonNode(ctx context.Context, nodeName string) error {
	log := log.FromContext(ctx).WithValues("node", nodeName, "scope", "UncordonNode")

	node := &corev1.Node{}
	if err := d.client.Get(ctx, ctrlclient.ObjectKey{Name: nodeName}, node); err != nil {
		return fmt.Errorf("failed to get node: %w", err)
	}

	patch := ctrlclient.StrategicMergeFrom(node, ctrlclient.MergeFromWithOptimisticLock{})

	newNode := node.DeepCopy()
	newNode.Spec.Unschedulable = false

	if err := d.client.Patch(ctx, newNode, patch); err != nil {
		return fmt.Errorf("failed to patch node: %w", err)
	}

	log.Info("Node uncordoned successfully")
	return nil
}

// DrainNode drains the specified node by evicting its pods according to the drain options.
func (d *drainer) DrainNode(ctx context.Context, nodeName string) error {
	logger := log.FromContext(ctx).WithValues("node", nodeName, "scope", "DrainNode")
	logger.Info("Starting node drain")

	ticker := time.NewTicker(d.opts.EvictionRetryInterval)
	drainCtx, cancel := context.WithTimeout(ctx, d.opts.Timeout)
	defer cancel()

	for {
		select {
		case <-drainCtx.Done():
			return drainCtx.Err()
		case <-ticker.C:
			logger.Info("Attempting to drain node")
		}

		podsToEvict, err := d.getPodsToEvict(ctx, nodeName)
		if err != nil {
			return fmt.Errorf("failed to get pods to evict from node %s: %w", nodeName, err)
		}

		if len(podsToEvict) == 0 {
			logger.Info("No pods to evict. Drain complete.")
			return nil
		}

		logger.Info("Evicting pods", "count", len(podsToEvict))

		evictCtx := drainCtx
		var cancel context.CancelFunc
		if d.opts.EvictionTimeout > 0 {
			evictCtx, cancel = context.WithTimeout(evictCtx, d.opts.EvictionTimeout)
		}

		d.evictOrDeletePods(evictCtx, podsToEvict)

		// Can not defer cancel here because of the loop
		if cancel != nil {
			cancel()
		}

		logger.Info("Pods evicted successfully, checking for remaining pods")
	}
}

// getPodsToEvict returns the list of pods on the given node that should be evicted
// based on the drain options.
func (d *drainer) getPodsToEvict(ctx context.Context, nodeName string) ([]corev1.Pod, error) {
	logger := log.FromContext(ctx).WithValues("node", nodeName, "scope", "getPodsToEvict")

	podList := &corev1.PodList{}
	if err := d.client.List(ctx, podList, &ctrlclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", nodeName),
	}); err != nil {
		return nil, fmt.Errorf("failed to list pods on node %s: %w", nodeName, err)
	}

	podsToEvict := make([]corev1.Pod, 0, len(podList.Items))
	for _, pod := range podList.Items {
		///
		// Skip static pods (those managed by kubelet directly)
		///

		if _, isStatic := pod.Annotations[corev1.MirrorPodAnnotationKey]; isStatic {
			logger.Info("Skipping static pod", "pod", pod.Name, "namespace", pod.Namespace)
			continue
		}

		///
		// Daemonsets
		///

		isDaemonSet := false
		for _, ownerRef := range pod.OwnerReferences {
			if ownerRef.Kind == "DaemonSet" {
				isDaemonSet = true
				break
			}
		}
		if isDaemonSet {
			if !d.opts.IgnoreDaemonsets {
				return nil, fmt.Errorf("pod %s/%s is managed by a DaemonSet; cannot drain node without IgnoreDaemonsets option", pod.Namespace, pod.Name)
			} else {
				logger.Info("Skipping DaemonSet pod", "pod", pod.Name, "namespace", pod.Namespace)
				continue
			}
		}

		///
		// emptyDir volumes
		///

		hasEmptyDir := false
		for _, volume := range pod.Spec.Volumes {
			if volume.EmptyDir != nil {
				hasEmptyDir = true
				break
			}
		}
		if !d.opts.DeleteEmptydirData && hasEmptyDir {
			// Do not continue if there are pods using emptyDir
			// (local data that will be deleted when the node is drained)
			return nil, fmt.Errorf("pod %s/%s is using emptyDir volume; cannot drain node without DeleteEmptydirData option", pod.Namespace, pod.Name)
		}

		///
		// Pods without controllers
		///

		hasController := false
		for _, ownerRef := range pod.OwnerReferences {
			if ownerRef.Controller != nil && *ownerRef.Controller {
				hasController = true
				break
			}
		}
		if !d.opts.Force && !hasController {
			return nil, fmt.Errorf("pod %s/%s does not have a controller; cannot drain node without Force option", pod.Namespace, pod.Name)
		}

		podsToEvict = append(podsToEvict, pod)
	}

	return podsToEvict, nil
}

// evictOrDeletePods evicts or deletes the given pods from the node.
// It first tries to evict the pods using the eviction API,
// and if that fails due to PodDisruptionBudget constraints, it deletes the pods if allowed.
// It also force deletes pods that are stuck in terminating state for longer than the grace period.
func (d *drainer) evictOrDeletePods(ctx context.Context, pods []corev1.Pod) {
	logger := log.FromContext(ctx).WithValues("scope", "evictOrDeletePods")

	for _, pod := range pods {
		podLog := logger.WithValues("pod", pod.Name, "namespace", pod.Namespace)

		// Force delete pods that are in terminating state for longer than the grace period
		if pod.DeletionTimestamp != nil {
			deletionDeadline := pod.DeletionTimestamp.Add(time.Duration(d.opts.GracePeriodSeconds) * time.Second)
			if d.nowFunc().After(deletionDeadline) {
				podLog.Info("Pod is stuck in terminating state for longer than the grace period, force deleting")

				// Remove finalizers to allow immediate deletion
				patch := ctrlclient.StrategicMergeFrom(&pod, ctrlclient.MergeFromWithOptimisticLock{})
				newPod := pod.DeepCopy()
				newPod.Finalizers = nil
				if err := d.client.Patch(ctx, newPod, patch); err != nil {
					podLog.Error(err, "Failed to remove finalizers from pod before force deletion")
					continue
				}

				err := d.client.Delete(ctx, &pod, &ctrlclient.DeleteOptions{
					GracePeriodSeconds: ptr.To(int64(0)),
				})
				if err != nil && !apierrors.IsNotFound(err) {
					podLog.Error(err, "Failed to force delete pod")
					continue
				}
				podLog.Info("Pod force deleted successfully")
				continue
			}
		}

		// Try to use eviction API first (respects PodDisruptionBudgets)
		eviction := &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
			DeleteOptions: &metav1.DeleteOptions{
				GracePeriodSeconds: &d.opts.GracePeriodSeconds,
			},
		}
		err := d.client.SubResource("eviction").Create(ctx, &pod, eviction)
		if err != nil {
			// Evictions are treated as “disruptions” that are rate-limited by a PDB.
			// When there’s no remaining budget, the API responds with 429 to signal a transient
			// condition: “try again later,” not a permanent denial.
			// 429 was chosen (instead of e.g. 403) so clients can back off and retry once budget becomes available.
			// https://kubernetes.io/docs/concepts/scheduling-eviction/api-eviction/#how-api-initiated-eviction-works
			if apierrors.IsTooManyRequests(err) {
				if d.opts.AllowDeletion {
					// PodDisruptionBudget is preventing eviction, delete instead
					podLog.Info("Eviction blocked by PDB, deleting pod")
					err = d.client.Delete(ctx, &pod, &ctrlclient.DeleteOptions{
						GracePeriodSeconds: &d.opts.GracePeriodSeconds,
					})
					if err != nil && !apierrors.IsNotFound(err) {
						podLog.Error(err, "Failed to delete pod")
						continue
					}
					podLog.Info("Pod deleted successfully")
					continue
				}
			} else if !apierrors.IsNotFound(err) {
				podLog.Error(err, "Failed to evict pod")
				continue
			}
		}

		podLog.Info("Pod eviction initiated")
	}
}

var _ Drainer = &drainer{}
