package ck8s

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	apiv1 "github.com/canonical/k8s-snap-api/api/v1"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	podv1 "k8s.io/kubernetes/pkg/api/v1/pod"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	controlplanev1 "github.com/canonical/cluster-api-k8s/controlplane/api/v1beta2"
	ck8serrors "github.com/canonical/cluster-api-k8s/pkg/errors"
)

const (
	// NOTE(neoaggelos): See notes below.
	labelNodeRoleControlPlane = "node-role.kubernetes.io/control-plane"
	k8sdConfigSecretName      = "k8sd-config" //nolint:gosec
)

// WorkloadCluster defines all behaviors necessary to upgrade kubernetes on a workload cluster
//
// TODO: Add a detailed description to each of these method definitions.
type WorkloadCluster interface {
	// Basic health and status checks.
	ClusterStatus(ctx context.Context) (ClusterStatus, error)
	UpdateAgentConditions(ctx context.Context, controlPlane *ControlPlane)
	NewControlPlaneJoinToken(ctx context.Context, name string) (string, error)
	NewWorkerJoinToken(ctx context.Context) (string, error)

	RemoveMachineFromCluster(ctx context.Context, machine *clusterv1.Machine) error
}

// Workload defines operations on workload clusters.
type Workload struct {
	WorkloadCluster
	authToken string

	Client              ctrlclient.Client
	ClientRestConfig    *rest.Config
	K8sdClientGenerator *k8sdClientGenerator
	microclusterPort    int
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
	}
	if err := w.Client.List(ctx, nodes, ctrlclient.MatchingLabels(labels)); err != nil {
		return nil, err
	}
	return nodes, nil
}

// ClusterStatus returns the status of the cluster.
func (w *Workload) ClusterStatus(ctx context.Context) (ClusterStatus, error) {
	status := ClusterStatus{}

	// NOTE(neoaggelos): Check that the k8sd-config on the kube-system configmap exists.
	key := ctrlclient.ObjectKey{
		Name:      k8sdConfigSecretName,
		Namespace: metav1.NamespaceSystem,
	}

	err := w.Client.Get(ctx, key, &corev1.ConfigMap{})
	// In case of error we do assume the control plane is not initialized yet.
	if err != nil {
		logger := log.FromContext(ctx)
		logger.Info("Control Plane does not seem to be initialized yet.", "reason", err.Error())
		status.HasK8sdConfigMap = false
		return status, err
	}

	status.HasK8sdConfigMap = true

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

	// Fetch the Pods only once.
	podmap, err := w.K8sdClientGenerator.getProxyPods(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get proxy pods: %w", err)
	}

	if len(podmap) == 0 {
		return nil, &ck8serrors.K8sdProxyNotFound{}
	}

	var allErrors []error
	for _, node := range cplaneNodes.Items {
		if _, ok := options.IgnoreNodes[node.Name]; ok {
			continue
		}

		pod, ok := podmap[node.Name]
		if !ok {
			allErrors = append(allErrors, fmt.Errorf("node %s has no k8sd proxy pod", node.Name))
			continue
		}

		if !podv1.IsPodReady(&pod) {
			// if the Pod is not Ready, it won't be able to accept any k8sd API calls.
			allErrors = append(allErrors, &ck8serrors.K8sdProxyNotReady{PodName: pod.Name})
			continue
		}

		proxy, err := w.K8sdClientGenerator.forNodePod(ctx, &node, pod.Name) // #nosec G601
		if err != nil {
			allErrors = append(allErrors, fmt.Errorf("could not create proxy client for node %s: %w", node.Name, err))
			continue
		}

		// Check if there is any response from the proxy.
		header := w.newHeaderWithCAPIAuthToken()
		if err := w.doK8sdRequest(ctx, proxy, http.MethodGet, "", header, "", nil); err != nil {
			allErrors = append(allErrors, fmt.Errorf("error while contacting proxy on node %s: %w", node.Name, err))
			continue
		}

		return proxy, nil
	}

	return nil, fmt.Errorf("failed to get k8sd proxy for control plane, previous errors: %w", errors.Join(allErrors...))
}

// GetK8sdProxyForMachine returns a k8sd proxy client for the machine.
func (w *Workload) GetK8sdProxyForMachine(ctx context.Context, machine *clusterv1.Machine) (*K8sdClient, error) {
	if machine == nil {
		return nil, fmt.Errorf("machine object is nil")
	}

	if machine.Status.NodeRef == nil {
		return nil, fmt.Errorf("machine %s has no node reference", machine.Name)
	}

	node := &corev1.Node{}
	if err := w.Client.Get(ctx, ctrlclient.ObjectKey{Name: machine.Status.NodeRef.Name}, node); err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	return w.K8sdClientGenerator.forNode(ctx, node)
}

func (w *Workload) GetCertificatesExpiryDate(ctx context.Context, machine *clusterv1.Machine, nodeToken string) (string, error) {
	request := apiv1.CertificatesExpiryRequest{}
	response := &apiv1.CertificatesExpiryResponse{}

	header := w.newHeaderWithNodeToken(nodeToken)
	k8sdProxy, err := w.GetK8sdProxyForMachine(ctx, machine)
	if err != nil {
		return "", fmt.Errorf("failed to create k8sd proxy: %w", err)
	}

	if err := w.doK8sdRequest(ctx, k8sdProxy, http.MethodPost, fmt.Sprintf("%s/%s", apiv1.K8sdAPIVersion, apiv1.ClusterAPICertificatesExpiryRPC), header, request, response); err != nil {
		return "", fmt.Errorf("failed to get certificates expiry date: %w", err)
	}

	return response.ExpiryDate, nil
}

func (w *Workload) ApproveCertificates(ctx context.Context, machine *clusterv1.Machine, seed int) error {
	request := apiv1.ClusterAPIApproveWorkerCSRRequest{
		Seed: seed,
	}
	response := &apiv1.ClusterAPIApproveWorkerCSRResponse{}
	k8sdProxy, err := w.GetK8sdProxyForControlPlane(ctx, k8sdProxyOptions{})
	if err != nil {
		return fmt.Errorf("failed to create k8sd proxy: %w", err)
	}

	header := w.newHeaderWithCAPIAuthToken()

	if err := w.doK8sdRequest(ctx, k8sdProxy, http.MethodPost, fmt.Sprintf("%s/%s", apiv1.K8sdAPIVersion, apiv1.ClusterAPIApproveWorkerCSRRPC), header, request, response); err != nil {
		return fmt.Errorf("failed to approve certificates: %w", err)
	}

	return nil
}

func (w *Workload) refreshCertificatesPlan(ctx context.Context, machine *clusterv1.Machine, nodeToken string) (int, error) {
	planRequest := apiv1.ClusterAPICertificatesPlanRequest{}
	planResponse := &apiv1.ClusterAPICertificatesPlanResponse{}

	header := w.newHeaderWithNodeToken(nodeToken)

	k8sdProxy, err := w.GetK8sdProxyForMachine(ctx, machine)
	if err != nil {
		return 0, fmt.Errorf("failed to create k8sd proxy: %w", err)
	}

	if err := w.doK8sdRequest(ctx, k8sdProxy, http.MethodPost, fmt.Sprintf("%s/%s", apiv1.K8sdAPIVersion, apiv1.ClusterAPICertificatesPlanRPC), header, planRequest, planResponse); err != nil {
		return 0, fmt.Errorf("failed to refresh certificates: %w", err)
	}

	return planResponse.Seed, nil
}

func (w *Workload) refreshCertificatesRun(ctx context.Context, machine *clusterv1.Machine, nodeToken string, request *apiv1.ClusterAPICertificatesRunRequest) (int, error) {
	runResponse := &apiv1.ClusterAPICertificatesRunResponse{}
	header := w.newHeaderWithNodeToken(nodeToken)

	k8sdProxy, err := w.GetK8sdProxyForMachine(ctx, machine)
	if err != nil {
		return 0, fmt.Errorf("failed to create k8sd proxy: %w", err)
	}

	if err := w.doK8sdRequest(ctx, k8sdProxy, http.MethodPost, fmt.Sprintf("%s/%s", apiv1.K8sdAPIVersion, apiv1.ClusterAPICertificatesRunRPC), header, request, runResponse); err != nil {
		return 0, fmt.Errorf("failed to run refresh certificates: %w", err)
	}

	return runResponse.ExpirationSeconds, nil
}

// RefreshWorkerCertificates approves the worker node CSR and refreshes the certificates.
// The certificate approval process follows these steps:
// 1. The CAPI provider calls the /x/capi/refresh-certs/plan endpoint from the
// worker node, which generates the CSRs and creates the CertificateSigningRequest
// objects in the cluster.
// 2. The CAPI provider then calls the /x/capi/refresh-certs/run endpoint with
// the seed. This endpoint waits until the CSR is approved and the certificate
// is signed. Note that this is a blocking call.
// 3. The CAPI provider calls the /x/capi/refresh-certs/approve endpoint from
// any control plane node to approve the CSRs.
// 4. The /x/capi/refresh-certs/run endpoint completes and returns once the
// certificate is approved and signed.
func (w *Workload) RefreshWorkerCertificates(ctx context.Context, machine *clusterv1.Machine, nodeToken string, expirationSeconds int) (int, error) {
	seed, err := w.refreshCertificatesPlan(ctx, machine, nodeToken)
	if err != nil {
		return 0, fmt.Errorf("failed to get refresh certificates plan: %w", err)
	}

	request := apiv1.ClusterAPICertificatesRunRequest{
		Seed:              seed,
		ExpirationSeconds: expirationSeconds,
	}

	var seconds int

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		seconds, err = w.refreshCertificatesRun(ctx, machine, nodeToken, &request)
		if err != nil {
			return fmt.Errorf("failed to run refresh certificates: %w", err)
		}
		return nil
	})

	eg.Go(func() error {
		if err := w.ApproveCertificates(ctx, machine, seed); err != nil {
			return fmt.Errorf("failed to approve certificates: %w", err)
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return 0, fmt.Errorf("failed to refresh worker certificates: %w", err)
	}

	return seconds, nil
}

func (w *Workload) RefreshControlPlaneCertificates(ctx context.Context, machine *clusterv1.Machine, nodeToken string, expirationSeconds int, extraSANs []string) (int, error) {
	seed, err := w.refreshCertificatesPlan(ctx, machine, nodeToken)
	if err != nil {
		return 0, fmt.Errorf("failed to get refresh certificates plan: %w", err)
	}

	runRequest := apiv1.ClusterAPICertificatesRunRequest{
		ExpirationSeconds: expirationSeconds,
		Seed:              seed,
		ExtraSANs:         extraSANs,
	}

	seconds, err := w.refreshCertificatesRun(ctx, machine, nodeToken, &runRequest)
	if err != nil {
		return 0, fmt.Errorf("failed to run refresh certificates: %w", err)
	}

	return seconds, nil
}

func (w *Workload) RefreshMachine(ctx context.Context, machine *clusterv1.Machine, nodeToken string, upgradeOption string) (string, error) {
	request := apiv1.SnapRefreshRequest{}
	response := &apiv1.SnapRefreshResponse{}
	optionKv := strings.Split(upgradeOption, "=")

	if len(optionKv) != 2 {
		return "", fmt.Errorf("invalid in-place upgrade release annotation: %s", upgradeOption)
	}

	switch optionKv[0] {
	case "channel":
		request.Channel = optionKv[1]
	case "revision":
		request.Revision = optionKv[1]
	case "localPath":
		request.LocalPath = optionKv[1]
	default:
		return "", fmt.Errorf("invalid upgrade option: %s", optionKv[0])
	}

	k8sdProxy, err := w.GetK8sdProxyForMachine(ctx, machine)
	if err != nil {
		return "", fmt.Errorf("failed to create k8sd proxy: %w", err)
	}

	header := w.newHeaderWithNodeToken(nodeToken)

	if err := w.doK8sdRequest(ctx, k8sdProxy, http.MethodPost, fmt.Sprintf("%s/%s", apiv1.K8sdAPIVersion, apiv1.SnapRefreshRPC), header, request, response); err != nil {
		return "", fmt.Errorf("failed to refresh machine %s: %w", machine.Name, err)
	}

	return response.ChangeID, nil
}

func (w *Workload) GetRefreshStatusForMachine(ctx context.Context, machine *clusterv1.Machine, nodeToken string, changeID string) (*apiv1.SnapRefreshStatusResponse, error) {
	request := apiv1.SnapRefreshStatusRequest{}
	response := &apiv1.SnapRefreshStatusResponse{}
	request.ChangeID = changeID

	k8sdProxy, err := w.GetK8sdProxyForMachine(ctx, machine)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8sd proxy: %w", err)
	}

	header := w.newHeaderWithNodeToken(nodeToken)

	if err := w.doK8sdRequest(ctx, k8sdProxy, http.MethodPost, fmt.Sprintf("%s/%s", apiv1.K8sdAPIVersion, apiv1.SnapRefreshStatusRPC), header, request, response); err != nil {
		return nil, fmt.Errorf("failed to refresh machine %s: %w", machine.Name, err)
	}

	return response, nil
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

	k8sdProxy, err := w.GetK8sdProxyForControlPlane(ctx, k8sdProxyOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create k8sd proxy: %w", err)
	}

	header := w.newHeaderWithCAPIAuthToken()

	if err := w.doK8sdRequest(ctx, k8sdProxy, http.MethodPost, fmt.Sprintf("%s/%s", apiv1.K8sdAPIVersion, apiv1.ClusterAPIGetJoinTokenRPC), header, request, response); err != nil {
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
	k8sdProxy, err := w.GetK8sdProxyForControlPlane(ctx, k8sdProxyOptions{IgnoreNodes: map[string]struct{}{nodeName: {}}})
	if err != nil {
		return fmt.Errorf("failed to create k8sd proxy: %w", err)
	}

	header := w.newHeaderWithCAPIAuthToken()

	if err := w.doK8sdRequest(ctx, k8sdProxy, http.MethodPost, fmt.Sprintf("%s/%s", apiv1.K8sdAPIVersion, apiv1.ClusterAPIRemoveNodeRPC), header, request, nil); err != nil {
		return fmt.Errorf("failed to remove %s from cluster: %w", machine.Name, err)
	}
	return nil
}

func (w *Workload) doK8sdRequest(ctx context.Context, k8sdProxy *K8sdClient, method, endpoint string, header map[string][]string, request any, response any) error {
	type wrappedResponse struct {
		Error    string          `json:"error"`
		Metadata json.RawMessage `json:"metadata"`
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

	req.Header = http.Header(header)

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
		// No response expected.
		return nil
	}
	if err := json.Unmarshal(responseBody.Metadata, response); err != nil {
		return fmt.Errorf("failed to parse HTTP response: %w", err)
	}

	return nil
}

// newHeaderWithCAPIAuthToken returns a map with the CAPI auth token as a header.
func (w *Workload) newHeaderWithCAPIAuthToken() map[string][]string {
	return map[string][]string{
		"capi-auth-token": {w.authToken},
	}
}

// newHeaderWithNodeToken returns a map with the node token as a header.
func (w *Workload) newHeaderWithNodeToken(nodeToken string) map[string][]string {
	return map[string][]string{
		"node-token": {nodeToken},
	}
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
		conditions.MarkFalse(input.controlPlane.KCP, input.condition, input.unhealthyReason, clusterv1.ConditionSeverityError, "%s", strings.Join(input.kcpErrors, "; "))
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

var _ WorkloadCluster = &Workload{}
