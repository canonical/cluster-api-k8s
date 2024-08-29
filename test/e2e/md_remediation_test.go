//go:build e2e
// +build e2e

/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var _ = Describe("When testing MachineDeployment remediation", func() {
	var (
		ctx                    = context.TODO()
		specName               = "machine-deployment-remediation"
		namespace              *corev1.Namespace
		cancelWatches          context.CancelFunc
		result                 *ApplyClusterTemplateAndWaitResult
		clusterName            string
		clusterctlLogFolder    string
		infrastructureProvider string
	)

	BeforeEach(func() {
		Expect(e2eConfig.Variables).To(HaveKey(KubernetesVersion))

		clusterName = fmt.Sprintf("capick8s-md-remediation-%s", util.RandomString(6))
		infrastructureProvider = "docker"

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, bootstrapClusterProxy, artifactFolder)

		result = new(ApplyClusterTemplateAndWaitResult)

		clusterctlLogFolder = filepath.Join(artifactFolder, "clusters", bootstrapClusterProxy.GetName())
	})

	AfterEach(func() {
		cleanInput := cleanupInput{
			SpecName:        specName,
			Cluster:         result.Cluster,
			ClusterProxy:    bootstrapClusterProxy,
			Namespace:       namespace,
			CancelWatches:   cancelWatches,
			IntervalsGetter: e2eConfig.GetIntervals,
			SkipCleanup:     skipCleanup,
			ArtifactFolder:  artifactFolder,
		}

		dumpSpecResourcesAndCleanup(ctx, cleanInput)
	})

	Context("Machine deployment remediation", func() {
		It("Should replace unhealthy machines", func() {
			By("Creating a workload cluster")
			ApplyClusterTemplateAndWait(ctx, ApplyClusterTemplateAndWaitInput{
				ClusterProxy: bootstrapClusterProxy,
				ConfigCluster: clusterctl.ConfigClusterInput{
					LogFolder:                clusterctlLogFolder,
					ClusterctlConfigPath:     clusterctlConfigPath,
					KubeconfigPath:           bootstrapClusterProxy.GetKubeconfigPath(),
					InfrastructureProvider:   infrastructureProvider,
					Flavor:                   "md-remediation",
					Namespace:                namespace.Name,
					ClusterName:              clusterName,
					KubernetesVersion:        e2eConfig.GetVariable(KubernetesVersion),
					ControlPlaneMachineCount: pointer.Int64Ptr(1),
					WorkerMachineCount:       pointer.Int64Ptr(1),
				},
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)

			// TODO: this should be re-written like the KCP remediation test, because the current implementation
			//   only tests that MHC applies the unhealthy condition but it doesn't test that the unhealthy machine is delete and a replacement machine comes up.
			By("Setting a machine unhealthy and wait for MachineDeployment remediation")
			framework.DiscoverMachineHealthChecksAndWaitForRemediation(ctx, framework.DiscoverMachineHealthCheckAndWaitForRemediationInput{
				ClusterProxy:              bootstrapClusterProxy,
				Cluster:                   result.Cluster,
				WaitForMachineRemediation: e2eConfig.GetIntervals(specName, "wait-machine-remediation"),
			})

			By("Waiting until nodes are ready")
			workloadProxy := bootstrapClusterProxy.GetWorkloadCluster(ctx, namespace.Name, result.Cluster.Name)
			workloadClient := workloadProxy.GetClient()

			WaitForNodesReady(ctx, WaitForNodesReadyInput{
				Lister:            workloadClient,
				KubernetesVersion: e2eConfig.GetVariable(KubernetesVersion),
				Count:             int(result.ExpectedTotalNodes()),
				WaitForNodesReady: e2eConfig.GetIntervals(specName, "wait-nodes-ready"),
			})

			By("PASSED!")
		})
	})
})

// DiscoverMachineHealthChecksAndWaitForRemediation patches an unhealthy node condition to one node observed by the Machine Health Check and then wait for remediation.
func DiscoverMachineHealthChecksAndWaitForRemediation(ctx context.Context, input framework.DiscoverMachineHealthCheckAndWaitForRemediationInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DiscoverMachineHealthChecksAndWaitForRemediation")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling DiscoverMachineHealthChecksAndWaitForRemediation")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling DiscoverMachineHealthChecksAndWaitForRemediation")

	mgmtClient := input.ClusterProxy.GetClient()
	fmt.Fprintln(GinkgoWriter, "Discovering machine health check resources")
	machineHealthChecks := framework.GetMachineHealthChecksForCluster(ctx, framework.GetMachineHealthChecksForClusterInput{
		Lister:      mgmtClient,
		ClusterName: input.Cluster.Name,
		Namespace:   input.Cluster.Namespace,
	})

	Expect(machineHealthChecks).NotTo(BeEmpty())

	for _, mhc := range machineHealthChecks {
		Expect(mhc.Spec.UnhealthyConditions).NotTo(BeEmpty())

		fmt.Fprintln(GinkgoWriter, "Ensuring there is at least 1 Machine that MachineHealthCheck is matching")
		machines := framework.GetMachinesByMachineHealthCheck(ctx, framework.GetMachinesByMachineHealthCheckInput{
			Lister:             mgmtClient,
			ClusterName:        input.Cluster.Name,
			MachineHealthCheck: mhc,
		})

		Expect(machines).NotTo(BeEmpty())

		fmt.Fprintln(GinkgoWriter, "Patching MachineHealthCheck unhealthy condition to one of the nodes")
		unhealthyNodeCondition := corev1.NodeCondition{
			Type:               mhc.Spec.UnhealthyConditions[0].Type,
			Status:             mhc.Spec.UnhealthyConditions[0].Status,
			LastTransitionTime: metav1.Time{Time: time.Now()},
		}
		framework.PatchNodeCondition(ctx, framework.PatchNodeConditionInput{
			ClusterProxy:  input.ClusterProxy,
			Cluster:       input.Cluster,
			NodeCondition: unhealthyNodeCondition,
			Machine:       machines[0],
		})

		fmt.Fprintln(GinkgoWriter, "Waiting for remediation x")
		framework.WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition(ctx, framework.WaitForMachineHealthCheckToRemediateUnhealthyNodeConditionInput{
			ClusterProxy:       input.ClusterProxy,
			Cluster:            input.Cluster,
			MachineHealthCheck: mhc,
			MachinesCount:      len(machines),
		}, input.WaitForMachineRemediation...)
	}
}

// WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition patches a node condition to any one of the machines with a node ref.
func WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition(ctx context.Context, input framework.WaitForMachineHealthCheckToRemediateUnhealthyNodeConditionInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition")
	Expect(input.MachineHealthCheck).NotTo(BeNil(), "Invalid argument. input.MachineHealthCheck can't be nil when calling WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition")
	Expect(input.MachinesCount).NotTo(BeZero(), "Invalid argument. input.MachinesCount can't be zero when calling WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition")

	fmt.Fprintln(GinkgoWriter, "Waiting until the node with unhealthy node condition is remediated")
	Eventually(func() bool {
		machines := framework.GetMachinesByMachineHealthCheck(ctx, framework.GetMachinesByMachineHealthCheckInput{
			Lister:             input.ClusterProxy.GetClient(),
			ClusterName:        input.Cluster.Name,
			MachineHealthCheck: input.MachineHealthCheck,
		})
		// Wait for all the machines to exist.
		// NOTE: this is required given that this helper is called after a remediation
		// and we want to make sure all the machine are back in place before testing for unhealthyCondition being fixed.
		fmt.Fprintf(GinkgoWriter, "waiting for all machines to exist, current count: %d, expected count: %d\n", len(machines), input.MachinesCount)
		if len(machines) < input.MachinesCount {
			return false
		}

		for _, machine := range machines {
			if machine.Status.NodeRef == nil {
				fmt.Fprintf(GinkgoWriter, "machine %s no node ref", machine.Name)
				return false
			}
			node := &corev1.Node{}
			// This should not be an Expect(), because it may return error during machine deletion.
			err := input.ClusterProxy.GetWorkloadCluster(ctx, input.Cluster.Namespace, input.Cluster.Name).GetClient().Get(ctx, types.NamespacedName{Name: machine.Status.NodeRef.Name, Namespace: machine.Status.NodeRef.Namespace}, node)
			if err != nil {
				fmt.Fprintf(GinkgoWriter, "failed to get node from ref: %v", err)
				return false
			}
			if hasMatchingUnhealthyConditions(input.MachineHealthCheck, node.Status.Conditions) {
				fmt.Fprintf(GinkgoWriter, "%s has not matching unhealthy condiditon", machine.Name)
				return false
			}
		}
		return true
	}, intervals...).Should(BeTrue())
}

// hasMatchingUnhealthyConditions returns true if any node condition matches with machine health check unhealthy conditions.
func hasMatchingUnhealthyConditions(machineHealthCheck *clusterv1.MachineHealthCheck, nodeConditions []corev1.NodeCondition) bool {
	fmt.Fprintf(GinkgoWriter, "checking for matching unhealthy conditions, machine health check: %v, node conditions: %v\n", machineHealthCheck.Spec.UnhealthyConditions, nodeConditions)
	for _, unhealthyCondition := range machineHealthCheck.Spec.UnhealthyConditions {
		for _, nodeCondition := range nodeConditions {
			if nodeCondition.Type == unhealthyCondition.Type && nodeCondition.Status == unhealthyCondition.Status {
				return true
			}
		}
	}
	return false
}
