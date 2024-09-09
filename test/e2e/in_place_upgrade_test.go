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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
)

var _ = Describe("In place upgrade", func() {
	var (
		ctx                    = context.TODO()
		specName               = "workload-cluster-inplace"
		namespace              *corev1.Namespace
		cancelWatches          context.CancelFunc
		result                 *ApplyClusterTemplateAndWaitResult
		clusterName            string
		clusterctlLogFolder    string
		infrastructureProvider string
	)

	BeforeEach(func() {
		Expect(e2eConfig.Variables).To(HaveKey(KubernetesVersion))

		clusterName = fmt.Sprintf("capick8s-in-place-%s", util.RandomString(6))
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

	Context("Performing in-place upgrades", func() {
		It("Creating a workload cluster and applying in-place upgrade to control-plane and worker machines [PR-Blocking]", func() {
			By("Creating a workload cluster of 1 control plane and 1 worker node")
			ApplyClusterTemplateAndWait(ctx, ApplyClusterTemplateAndWaitInput{
				ClusterProxy: bootstrapClusterProxy,
				ConfigCluster: clusterctl.ConfigClusterInput{
					LogFolder:                clusterctlLogFolder,
					ClusterctlConfigPath:     clusterctlConfigPath,
					KubeconfigPath:           bootstrapClusterProxy.GetKubeconfigPath(),
					InfrastructureProvider:   infrastructureProvider,
					Namespace:                namespace.Name,
					ClusterName:              clusterName,
					KubernetesVersion:        e2eConfig.GetVariable(KubernetesVersion),
					ControlPlaneMachineCount: ptr.To(int64(1)),
					WorkerMachineCount:       ptr.To(int64(1)),
				},
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)

			bootstrapProxyClient := bootstrapClusterProxy.GetClient()

			By("Applying in place upgrade with local path for control plane nodes")
			ApplyInPlaceUpgradeForControlPlane(ctx, ApplyInPlaceUpgradeForControlPlaneInput{
				Lister:                  bootstrapProxyClient,
				Getter:                  bootstrapProxyClient,
				ClusterProxy:            bootstrapClusterProxy,
				Cluster:                 result.Cluster,
				WaitForUpgradeIntervals: e2eConfig.GetIntervals(specName, "wait-machine-upgrade"),
			})

			By("Applying in place upgrade with local path for worker nodes")
			ApplyInPlaceUpgradeForWorker(ctx, ApplyInPlaceUpgradeForWorkerInput{
				Lister:                  bootstrapProxyClient,
				Getter:                  bootstrapProxyClient,
				ClusterProxy:            bootstrapClusterProxy,
				Cluster:                 result.Cluster,
				WaitForUpgradeIntervals: e2eConfig.GetIntervals(specName, "wait-machine-upgrade"),
				MachineDeployments:      result.MachineDeployments,
			})
		})
	})

})
