package ck8s

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	apiv1 "github.com/canonical/k8s-snap-api/api/v1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	controlplanev1 "github.com/canonical/cluster-api-k8s/controlplane/api/v1beta2"
)

const (
	// NOTE(neoaggelos): See notes below.
	labelNodeRoleControlPlane = "node-role.kubernetes.io/control-plane"
	k8sdConfigSecretName      = "k8sd-config" //nolint:gosec
)

var (
	ErrControlPlaneMinNodes = errors.New("cluster has fewer than 2 control plane nodes; removing an etcd member is not supported")
)

// WorkloadCluster defines all behaviors necessary to upgrade kubernetes on a workload cluster
//
// TODO: Add a detailed description to each of these method definitions.
type WorkloadCluster interface {
	// Basic health and status checks.
	ClusterStatus(ctx context.Context) (ClusterStatus, error)
	UpdateAgentConditions(ctx context.Context, controlPlane *ControlPlane)
	UpdateEtcdConditions(ctx context.Context, controlPlane *ControlPlane)
	NewControlPlaneJoinToken(ctx context.Context, name string) (string, error)
	NewWorkerJoinToken(ctx context.Context) (string, error)

	RemoveMachineFromCluster(ctx context.Context, machine *clusterv1.Machine) error

	// NOTE(neoaggelos): See notes in (*CK8sControlPlaneReconciler).reconcileEtcdMembers
	//
	// TODO(neoaggelos): Replace with operations that use the k8sd proxy with things we need. For example, the function to remove a node _could_ be:
	//
	// 		RemoveMachineFromCluster(ctx context.Context, machine *clusterv1.Machine)
	//
	// Then, the implementation of WorkloadCluster should handle everything (reaching to k8sd, calling the right endpoints, authenticating, etc)
	// internally.
	/**
	// Etcd tasks
	RemoveEtcdMemberForMachine(ctx context.Context, machine *clusterv1.Machine) (bool, error)
	ForwardEtcdLeadership(ctx context.Context, machine *clusterv1.Machine, leaderCandidate *clusterv1.Machine) error
	ReconcileEtcdMembers(ctx context.Context, nodeNames []string) ([]string, error)
	**/

	// AllowBootstrapTokensToGetNodes(ctx context.Context) error
}

// Workload defines operations on workload clusters.
type Workload struct {
	WorkloadCluster
	authToken           string
	Client              ctrlclient.Client
	ClientRestConfig    *rest.Config
	K8sdClientGenerator *k8sdClientGenerator
	microclusterPort    int

	// NOTE(neoaggelos): CoreDNSMigrator and etcdClientGenerator are used by upstream to reach and manage the services in the workload cluster
	// TODO(neoaggelos): Replace them with a k8sdProxyClientGenerator.
	/**
	CoreDNSMigrator     coreDNSMigrator
	etcdClientGenerator etcdClientFor
	**/
}

// ClusterStatus holds stats information about the cluster.
type ClusterStatus struct {
	// Nodes are a total count of nodes
	Nodes int32
	// ReadyNodes are the count of nodes that are reporting ready
	ReadyNodes int32
	// HasK8sdConfigMap will be true if the k8sd-config configmap has been uploaded, false otherwise.
	HasK8sdConfigMap bool
}

func (w *Workload) getControlPlaneNodes(ctx context.Context) (*corev1.NodeList, error) {
	nodes := &corev1.NodeList{}
	labels := map[string]string{
		// NOTE(neoaggelos): Canonical Kubernetes uses node-role.kubernetes.io/control-plane="" as a label for control plane nodes.
		labelNodeRoleControlPlane: "",
		/**
		labelNodeRoleControlPlane: "true",
		**/
	}
	if err := w.Client.List(ctx, nodes, ctrlclient.MatchingLabels(labels)); err != nil {
		return nil, err
	}
	return nodes, nil
}

// ClusterStatus returns the status of the cluster.
func (w *Workload) ClusterStatus(ctx context.Context) (ClusterStatus, error) {
	status := ClusterStatus{}

	// count the control plane nodes
	nodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return status, err
	}

	status.Nodes = int32(len(nodes.Items))
	for _, node := range nodes.Items {
		nodeCopy := node
		if util.IsNodeReady(&nodeCopy) {
			status.ReadyNodes++
		}
	}

	// NOTE(neoaggelos): Check that the k8sd-config on the kube-system configmap exists.
	key := ctrlclient.ObjectKey{
		Name:      k8sdConfigSecretName,
		Namespace: metav1.NamespaceSystem,
	}

	err = w.Client.Get(ctx, key, &corev1.ConfigMap{})
	// In case of error we do assume the control plane is not initialized yet.
	if err != nil {
		logger := log.FromContext(ctx)
		logger.Info("Control Plane does not seem to be initialized yet.", "reason", err.Error())
	}

	status.HasK8sdConfigMap = err == nil

	return status, nil
}

func hasProvisioningMachine(machines collections.Machines) bool {
	for _, machine := range machines {
		if machine.Status.NodeRef == nil {
			return true
		}
	}
	return false
}

// nodeHasUnreachableTaint returns true if the node has is unreachable from the node controller.
func nodeHasUnreachableTaint(node corev1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == corev1.TaintNodeUnreachable && taint.Effect == corev1.TaintEffectNoExecute {
			return true
		}
	}
	return false
}

func getNodeInternalIP(node *corev1.Node) (string, error) {
	// TODO: Make this more robust by possibly finding/parsing the right IP.
	// This works as a start but might not be sufficient as the kubelet IP might not match microcluster IP.
	for _, addr := range node.Status.Addresses {
		if addr.Type == "InternalIP" {
			return addr.Address, nil
		}
	}

	return "", fmt.Errorf("unable to find internal IP for node %s", node.Name)
}

type k8sdProxyOptions struct {
	// IgnoreNodes is a set of node names that should be ignored when selecting a control plane node to proxy to.
	// This is useful when a control plane node is being removed and we want to avoid using it
	IgnoreNodes map[string]struct{}
}

// GetK8sdProxyForControlPlane returns a k8sd proxy client for the control plane.
func (w *Workload) GetK8sdProxyForControlPlane(ctx context.Context, options k8sdProxyOptions) (*K8sdClient, error) {
	cplaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get control plane nodes: %w", err)
	}

	for _, node := range cplaneNodes.Items {
		if _, ok := options.IgnoreNodes[node.Name]; ok {
			continue
		}

		proxy, err := w.K8sdClientGenerator.forNode(ctx, &node) // #nosec G601
		if err != nil {
			continue
		}

		return proxy, nil
	}

	return nil, fmt.Errorf("failed to get k8sd proxy for control plane")
}

// NewControlPlaneJoinToken creates a new join token for a control plane node.
// NewControlPlaneJoinToken reaches out to the control-plane of the workload cluster via k8sd-proxy client.
func (w *Workload) NewControlPlaneJoinToken(ctx context.Context, name string) (string, error) {
	return w.requestJoinToken(ctx, name, false)
}

// NewWorkerJoinToken creates a new join token for a worker node.
// NewWorkerJoinToken reaches out to the control-plane of the workload cluster via k8sd-proxy client.
func (w *Workload) NewWorkerJoinToken(ctx context.Context) (string, error) {
	// Accept any hostname by passing an empty string
	// Some infrastructures will have machines where hostname and machine name do not match by design (e.g. AWS)
	return w.requestJoinToken(ctx, "", true)
}

// requestJoinToken requests a join token from the existing control-plane nodes via the k8sd proxy.
func (w *Workload) requestJoinToken(ctx context.Context, name string, worker bool) (string, error) {
	request := apiv1.GetJoinTokenRequest{Name: name, Worker: worker}
	response := &apiv1.GetJoinTokenResponse{}
	err := w.doK8sdRequest(ctx, http.MethodPost, "1.0/x/capi/generate-join-token", request, response, k8sdProxyOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get join token: %w", err)
	}
	return response.EncodedToken, nil
}

func (w *Workload) RemoveMachineFromCluster(ctx context.Context, machine *clusterv1.Machine) error {
	if machine == nil {
		return fmt.Errorf("machine object is not set")
	}
	if machine.Status.NodeRef == nil {
		return fmt.Errorf("machine %s has no node reference", machine.Name)
	}

	nodeName := machine.Status.NodeRef.Name
	request := &apiv1.RemoveNodeRequest{Name: nodeName, Force: true}

	// If we see that ignoring control-planes is causing issues, let's consider removing it.
	// It *should* not be necessary as a machine should be able to remove itself from the cluster.
	err := w.doK8sdRequest(ctx, http.MethodPost, "1.0/x/capi/remove-node", request, nil, k8sdProxyOptions{IgnoreNodes: map[string]struct{}{nodeName: {}}})
	if err != nil {
		return fmt.Errorf("failed to remove %s from cluster: %w", machine.Name, err)
	}
	return nil
}

func (w *Workload) doK8sdRequest(ctx context.Context, method, endpoint string, request any, response any, k8sdProxyOptions k8sdProxyOptions) error {
	type wrappedResponse struct {
		Error    string          `json:"error"`
		Metadata json.RawMessage `json:"metadata"`
	}

	k8sdProxy, err := w.GetK8sdProxyForControlPlane(ctx, k8sdProxyOptions)
	if err != nil {
		return fmt.Errorf("failed to create k8sd proxy: %w", err)
	}

	url := fmt.Sprintf("https://%s:%v/%s", k8sdProxy.NodeIP, w.microclusterPort, endpoint)

	requestBody, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to prepare worker info request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(requestBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("capi-auth-token", w.authToken)
	res, err := k8sdProxy.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach k8sd through proxy client: %w", err)
	}
	defer res.Body.Close()

	var responseBody wrappedResponse
	if err := json.NewDecoder(res.Body).Decode(&responseBody); err != nil {
		return fmt.Errorf("failed to parse HTTP response: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request failed with status code: %d (%s)", res.StatusCode, responseBody.Error)
	}
	if responseBody.Error != "" {
		return fmt.Errorf("k8sd request failed: %s", responseBody.Error)
	}
	if responseBody.Metadata == nil || response == nil {
		// Nothing to decode
		return nil
	}
	if err := json.Unmarshal(responseBody.Metadata, response); err != nil {
		return fmt.Errorf("failed to parse HTTP response: %w", err)
	}

	return nil
}

// UpdateAgentConditions is responsible for updating machine conditions reflecting the status of all the control plane
// components. This operation is best effort, in the sense that in case
// of problems in retrieving the pod status, it sets the condition to Unknown state without returning any error.
func (w *Workload) UpdateAgentConditions(ctx context.Context, controlPlane *ControlPlane) {
	allMachinePodConditions := []clusterv1.ConditionType{
		controlplanev1.MachineAgentHealthyCondition,
	}

	// NOTE: this fun uses control plane nodes from the workload cluster as a source of truth for the current state.
	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		for i := range controlPlane.Machines {
			machine := controlPlane.Machines[i]
			for _, condition := range allMachinePodConditions {
				conditions.MarkUnknown(machine, condition, controlplanev1.PodInspectionFailedReason, "Failed to get the node which is hosting this component")
			}
		}
		conditions.MarkUnknown(controlPlane.KCP, controlplanev1.ControlPlaneComponentsHealthyCondition, controlplanev1.ControlPlaneComponentsInspectionFailedReason, "Failed to list nodes which are hosting control plane components")
		return
	}

	// Update conditions for control plane components hosted as static pods on the nodes.
	var kcpErrors []string

	for _, node := range controlPlaneNodes.Items {
		// Search for the machine corresponding to the node.
		var machine *clusterv1.Machine
		for _, m := range controlPlane.Machines {
			if m.Status.NodeRef != nil && m.Status.NodeRef.Name == node.Name {
				machine = m
				break
			}
		}

		// If there is no machine corresponding to a node, determine if this is an error or not.
		if machine == nil {
			// If there are machines still provisioning there is the chance that a chance that a node might be linked to a machine soon,
			// otherwise report the error at KCP level given that there is no machine to report on.
			if hasProvisioningMachine(controlPlane.Machines) {
				continue
			}
			kcpErrors = append(kcpErrors, fmt.Sprintf("Control plane node %s does not have a corresponding machine", node.Name))
			continue
		}

		// If the machine is deleting, report all the conditions as deleting
		if !machine.ObjectMeta.DeletionTimestamp.IsZero() {
			for _, condition := range allMachinePodConditions {
				conditions.MarkFalse(machine, condition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
			}
			continue
		}

		// If the node is Unreachable, information about static pods could be stale so set all conditions to unknown.
		if nodeHasUnreachableTaint(node) {
			// NOTE: We are assuming unreachable as a temporary condition, leaving to MHC
			// the responsibility to determine if the node is unhealthy or not.
			for _, condition := range allMachinePodConditions {
				conditions.MarkUnknown(machine, condition, controlplanev1.PodInspectionFailedReason, "Node is unreachable")
			}
			continue
		}

		targetnode := corev1.Node{}
		nodeKey := ctrlclient.ObjectKey{
			Namespace: metav1.NamespaceSystem,
			Name:      node.Name,
		}

		if err := w.Client.Get(ctx, nodeKey, &targetnode); err != nil {
			// If there is an error getting the Pod, do not set any conditions.
			if apierrors.IsNotFound(err) {
				conditions.MarkFalse(machine, controlplanev1.MachineAgentHealthyCondition, controlplanev1.PodMissingReason, clusterv1.ConditionSeverityError, "Node %s is missing", nodeKey.Name)

				return
			}
			conditions.MarkUnknown(machine, controlplanev1.MachineAgentHealthyCondition, controlplanev1.PodInspectionFailedReason, "Failed to get node status")
			return
		}

		for _, condition := range targetnode.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				conditions.MarkTrue(machine, controlplanev1.MachineAgentHealthyCondition)
			}
		}
	}

	// If there are provisioned machines without corresponding nodes, report this as a failing conditions with SeverityError.
	for i := range controlPlane.Machines {
		machine := controlPlane.Machines[i]
		if machine.Status.NodeRef == nil {
			continue
		}
		found := false
		for _, node := range controlPlaneNodes.Items {
			if machine.Status.NodeRef.Name == node.Name {
				found = true
				break
			}
		}
		if !found {
			for _, condition := range allMachinePodConditions {
				conditions.MarkFalse(machine, condition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "Missing node")
			}
		}
	}

	// Aggregate components error from machines at KCP level.
	aggregateFromMachinesToKCP(aggregateFromMachinesToKCPInput{
		controlPlane:      controlPlane,
		machineConditions: allMachinePodConditions,
		kcpErrors:         kcpErrors,
		condition:         controlplanev1.ControlPlaneComponentsHealthyCondition,
		unhealthyReason:   controlplanev1.ControlPlaneComponentsUnhealthyReason,
		unknownReason:     controlplanev1.ControlPlaneComponentsUnknownReason,
		note:              "control plane",
	})
}

type aggregateFromMachinesToKCPInput struct {
	controlPlane      *ControlPlane
	machineConditions []clusterv1.ConditionType
	kcpErrors         []string
	condition         clusterv1.ConditionType
	unhealthyReason   string
	unknownReason     string
	note              string
}

// aggregateFromMachinesToKCP aggregates a group of conditions from machines to KCP.
// NOTE: this func follows the same aggregation rules used by conditions.Merge thus giving priority to
// errors, then warning, info down to unknown.
func aggregateFromMachinesToKCP(input aggregateFromMachinesToKCPInput) {
	// Aggregates machines for condition status.
	// NB. A machine could be assigned to many groups, but only the group with the highest severity will be reported.
	kcpMachinesWithErrors := sets.NewString()
	kcpMachinesWithWarnings := sets.NewString()
	kcpMachinesWithInfo := sets.NewString()
	kcpMachinesWithTrue := sets.NewString()
	kcpMachinesWithUnknown := sets.NewString()

	for i := range input.controlPlane.Machines {
		machine := input.controlPlane.Machines[i]
		for _, condition := range input.machineConditions {
			if machineCondition := conditions.Get(machine, condition); machineCondition != nil {
				switch machineCondition.Status {
				case corev1.ConditionTrue:
					kcpMachinesWithTrue.Insert(machine.Name)
				case corev1.ConditionFalse:
					switch machineCondition.Severity {
					case clusterv1.ConditionSeverityInfo:
						kcpMachinesWithInfo.Insert(machine.Name)
					case clusterv1.ConditionSeverityWarning:
						kcpMachinesWithWarnings.Insert(machine.Name)
					case clusterv1.ConditionSeverityError:
						kcpMachinesWithErrors.Insert(machine.Name)
					}
				case corev1.ConditionUnknown:
					kcpMachinesWithUnknown.Insert(machine.Name)
				}
			}
		}
	}

	// In case of at least one machine with errors or KCP level errors (nodes without machines), report false, error.
	if len(kcpMachinesWithErrors) > 0 {
		input.kcpErrors = append(input.kcpErrors, fmt.Sprintf("Following machines are reporting %s errors: %s", input.note, strings.Join(kcpMachinesWithErrors.List(), ", ")))
	}
	if len(input.kcpErrors) > 0 {
		conditions.MarkFalse(input.controlPlane.KCP, input.condition, input.unhealthyReason, clusterv1.ConditionSeverityError, strings.Join(input.kcpErrors, "; "))
		return
	}

	// In case of no errors and at least one machine with warnings, report false, warnings.
	if len(kcpMachinesWithWarnings) > 0 {
		conditions.MarkFalse(input.controlPlane.KCP, input.condition, input.unhealthyReason, clusterv1.ConditionSeverityWarning, "Following machines are reporting %s warnings: %s", input.note, strings.Join(kcpMachinesWithWarnings.List(), ", "))
		return
	}

	// In case of no errors, no warning, and at least one machine with info, report false, info.
	if len(kcpMachinesWithWarnings) > 0 {
		conditions.MarkFalse(input.controlPlane.KCP, input.condition, input.unhealthyReason, clusterv1.ConditionSeverityWarning, "Following machines are reporting %s info: %s", input.note, strings.Join(kcpMachinesWithInfo.List(), ", "))
		return
	}

	// In case of no errors, no warning, no Info, and at least one machine with true conditions, report true.
	if len(kcpMachinesWithTrue) > 0 {
		conditions.MarkTrue(input.controlPlane.KCP, input.condition)
		return
	}

	// Otherwise, if there is at least one machine with unknown, report unknown.
	if len(kcpMachinesWithUnknown) > 0 {
		conditions.MarkUnknown(input.controlPlane.KCP, input.condition, input.unknownReason, "Following machines are reporting unknown %s status: %s", input.note, strings.Join(kcpMachinesWithUnknown.List(), ", "))
		return
	}

	// This last case should happen only if there are no provisioned machines, and thus without conditions.
	// So there will be no condition at KCP level too.
}

// UpdateEtcdConditions is responsible for updating machine conditions reflecting the status of all the etcd members.
// This operation is best effort, in the sense that in case of problems in retrieving member status, it sets
// the condition to Unknown state without returning any error.
func (w *Workload) UpdateEtcdConditions(ctx context.Context, controlPlane *ControlPlane) {
	w.updateManagedEtcdConditions(ctx, controlPlane)
}

func (w *Workload) updateManagedEtcdConditions(ctx context.Context, controlPlane *ControlPlane) {
	// NOTE: This methods uses control plane nodes only to get in contact with etcd but then it relies on etcd
	// as ultimate source of truth for the list of members and for their health.
	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		conditions.MarkUnknown(controlPlane.KCP, controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterInspectionFailedReason, "Failed to list nodes which are hosting the etcd members")
		for _, m := range controlPlane.Machines {
			conditions.MarkUnknown(m, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberInspectionFailedReason, "Failed to get the node which is hosting the etcd member")
		}
		return
	}

	// NOTE(neoaggelos): Upstream queries the etcd cluster endpoint on each of the machine nodes. It verifies that the list of etcd peers retrieved by
	// every node in the cluster matches with other nodes, and also verifies that they report the same etcd cluster ID.
	//
	// In the case of k8s-dqlite, we should do similar steps against the k8s-dqlite cluster. Until that is implemented, we skip this check and assume
	// that the node's datastore is in healthy condition if there are matching clusterv1.Machine and corev1.Node objects.
	//
	// TODO(neoaggelos): Implement API endpoints in k8sd to reach the local k8s-dqlite node and report the known cluster members. Then, verify that the
	// list of members matches across all the nodes.

	// Update conditions for etcd members on the nodes.
	var (
		// kcpErrors is used to store errors that can't be reported on any machine.
		kcpErrors []string
		/**
		// clusterID is used to store and compare the etcd's cluster id.
		clusterID *uint64
		// members is used to store the list of etcd members and compare with all the other nodes in the cluster.
		members []*etcd.Member
		**/
	)

	for _, node := range controlPlaneNodes.Items {
		// Search for the machine corresponding to the node.
		var machine *clusterv1.Machine
		for _, m := range controlPlane.Machines {
			if m.Status.NodeRef != nil && m.Status.NodeRef.Name == node.Name {
				machine = m
			}
		}

		if machine == nil {
			// If there are machines still provisioning there is the chance that a chance that a node might be linked to a machine soon,
			// otherwise report the error at KCP level given that there is no machine to report on.
			if hasProvisioningMachine(controlPlane.Machines) {
				continue
			}
			kcpErrors = append(kcpErrors, fmt.Sprintf("Control plane node %s does not have a corresponding machine", node.Name))
			continue
		}

		// If the machine is deleting, report all the conditions as deleting
		if !machine.ObjectMeta.DeletionTimestamp.IsZero() {
			conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
			continue
		}

		/**
		currentMembers, err := w.getCurrentEtcdMembers(ctx, machine, node.Name)
		if err != nil {
			continue
		}

		// Check if the list of members IDs reported is the same as all other members.
		// NOTE: the first member reporting this information is the baseline for this information.
		if members == nil {
			members = currentMembers
		}
		if !etcdutil.MemberEqual(members, currentMembers) {
			conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "etcd member reports the cluster is composed by members %s, but all previously seen etcd members are reporting %s", etcdutil.MemberNames(currentMembers), etcdutil.MemberNames(members))
			continue
		}

		// Retrieve the member and check for alarms.
		// NB. The member for this node always exists given forFirstAvailableNode(node) used above
		member := etcdutil.MemberForName(currentMembers, node.Name)
		if member == nil {
			conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "etcd member reports the cluster is composed by members %s, but the member itself (%s) is not included", etcdutil.MemberNames(currentMembers), node.Name)
			continue
		}
		if len(member.Alarms) > 0 {
			alarmList := []string{}
			for _, alarm := range member.Alarms {
				switch alarm {
				case etcd.AlarmOK:
					continue
				default:
					alarmList = append(alarmList, etcd.AlarmTypeName[alarm])
				}
			}
			if len(alarmList) > 0 {
				conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Etcd member reports alarms: %s", strings.Join(alarmList, ", "))
				continue
			}
		}

		// Check if the member belongs to the same cluster as all other members.
		// NOTE: the first member reporting this information is the baseline for this information.
		if clusterID == nil {
			clusterID = &member.ClusterID
		}
		if *clusterID != member.ClusterID {
			conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "etcd member has cluster ID %d, but all previously seen etcd members have cluster ID %d", member.ClusterID, *clusterID)
			continue
		}
		**/

		conditions.MarkTrue(machine, controlplanev1.MachineEtcdMemberHealthyCondition)
	}

	/**
	// Make sure that the list of etcd members and machines is consistent.
	kcpErrors = compareMachinesAndMembers(controlPlane, members, kcpErrors)
	**/

	// Aggregate components error from machines at KCP level
	aggregateFromMachinesToKCP(aggregateFromMachinesToKCPInput{
		controlPlane:      controlPlane,
		machineConditions: []clusterv1.ConditionType{controlplanev1.MachineEtcdMemberHealthyCondition},
		kcpErrors:         kcpErrors,
		condition:         controlplanev1.EtcdClusterHealthyCondition,
		unhealthyReason:   controlplanev1.EtcdClusterUnhealthyReason,
		unknownReason:     controlplanev1.EtcdClusterUnknownReason,
		note:              "etcd member",
	})
}

//nolint:godot
/**
func (w *Workload) getCurrentEtcdMembers(ctx context.Context, machine *clusterv1.Machine, nodeName string) ([]*etcd.Member, error) {
	// Create the etcd Client for the etcd Pod scheduled on the Node
	etcdClient, err := w.etcdClientGenerator.forFirstAvailableNode(ctx, []string{nodeName})
	if err != nil {
		conditions.MarkUnknown(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberInspectionFailedReason, "Failed to connect to the etcd pod on the %s node: %s", nodeName, err)
		return nil, errors.Wrapf(err, "failed to get current etcd members: failed to connect to the etcd pod on the %s node", nodeName)
	}
	defer etcdClient.Close()

	// While creating a new client, forFirstAvailableNode retrieves the status for the endpoint; check if the endpoint has errors.
	if len(etcdClient.Errors) > 0 {
		conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Etcd member status reports errors: %s", strings.Join(etcdClient.Errors, ", "))
		return nil, errors.Errorf("failed to get current etcd members: etcd member status reports errors: %s", strings.Join(etcdClient.Errors, ", "))
	}

	// Gets the list etcd members known by this member.
	currentMembers, err := etcdClient.Members(ctx)
	if err != nil {
		// NB. We should never be in here, given that we just received answer to the etcd calls included in forFirstAvailableNode;
		// however, we are considering the calls to Members a signal of etcd not being stable.
		conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Failed get answer from the etcd member on the %s node", nodeName)
		return nil, errors.Errorf("failed to get current etcd members: failed get answer from the etcd member on the %s node", nodeName)
	}

	return currentMembers, nil
}

func compareMachinesAndMembers(controlPlane *ControlPlane, members []*etcd.Member, kcpErrors []string) []string {
	// NOTE: We run this check only if we actually know the list of members, otherwise the first for loop
	// could generate a false negative when reporting missing etcd members.
	if members == nil {
		return kcpErrors
	}

	// Check Machine -> Etcd member.
	for _, machine := range controlPlane.Machines {
		if machine.Status.NodeRef == nil {
			continue
		}
		found := false
		for _, member := range members {
			nodeNameFromMember := etcdutil.NodeNameFromMember(member)
			if machine.Status.NodeRef.Name == nodeNameFromMember {
				found = true
				break
			}
		}
		if !found {
			conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Missing etcd member")
		}
	}

	// Check Etcd member -> Machine.
	for _, member := range members {
		found := false
		nodeNameFromMember := etcdutil.NodeNameFromMember(member)
		for _, machine := range controlPlane.Machines {
			if machine.Status.NodeRef != nil && machine.Status.NodeRef.Name == nodeNameFromMember {
				found = true
				break
			}
		}
		if !found {
			name := nodeNameFromMember
			if name == "" {
				name = fmt.Sprintf("%d (Name not yet assigned)", member.ID)
			}
			kcpErrors = append(kcpErrors, fmt.Sprintf("etcd member %s does not have a corresponding machine", name))
		}
	}
	return kcpErrors
}

func generateClientCert(caCertEncoded, caKeyEncoded []byte) (tls.Certificate, error) {
	// TODO: need to cache clientkey to clusterCacheTracker to avoid recreating key frequently
	clientKey, err := certs.NewPrivateKey()
	if err != nil {
		return tls.Certificate{}, errors.Wrapf(err, "error creating client key")
	}

	caCert, err := certs.DecodeCertPEM(caCertEncoded)
	if err != nil {
		return tls.Certificate{}, err
	}
	caKey, err := certs.DecodePrivateKeyPEM(caKeyEncoded)
	if err != nil {
		return tls.Certificate{}, err
	}
	x509Cert, err := newClientCert(caCert, clientKey, caKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(certs.EncodeCertPEM(x509Cert), certs.EncodePrivateKeyPEM(clientKey))
}

func newClientCert(caCert *x509.Certificate, key *rsa.PrivateKey, caKey crypto.Signer) (*x509.Certificate, error) {
	cfg := certs.Config{
		CommonName: "cluster-api.x-k8s.io",
	}

	now := time.Now().UTC()

	tmpl := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		NotBefore:   now.Add(time.Minute * -5),
		NotAfter:    now.Add(time.Hour * 24 * 365 * 10), // 10 years
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	b, err := x509.CreateCertificate(rand.Reader, &tmpl, caCert, key.Public(), caKey)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create signed client certificate: %+v", tmpl)
	}

	c, err := x509.ParseCertificate(b)
	return c, errors.WithStack(err)
}
**/

var _ WorkloadCluster = &Workload{}
