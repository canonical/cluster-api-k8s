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

	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Certificate Refresh", func() {
	var (
		ctx                    = context.TODO()
		specName               = "workload-cluster-certificate-refresh"
		namespace              *corev1.Namespace
		cancelWatches          context.CancelFunc
		result                 *ApplyClusterTemplateAndWaitResult
		clusterName            string
		clusterctlLogFolder    string
		infrastructureProvider string
	)

	BeforeEach(func() {
		Expect(e2eConfig.Variables).To(HaveKey(KubernetesVersion))

		clusterName = fmt.Sprintf("capick8s-certificate-refresh-%s", util.RandomString(6))
		infrastructureProvider = clusterctl.DefaultInfrastructureProvider

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, bootstrapClusterProxy, artifactFolder)

		// Create LXC secret for LXD provider if needed
		createLXCSecret(ctx, bootstrapClusterProxy, e2eConfig, namespace.Name)

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

	Context("Performing certificate refresh", func() {
		It("Should successfully refresh certificates for a cluster [PR-Blocking]", func() {
			By("Creating a workload cluster with a single control plane and a single worker node")
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

			By("Refreshing certificates for the control plane nodes")
			ApplyCertificateRefreshForControlPlane(ctx, ApplyCertificateRefreshForControlPlaneInput{
				Lister:                  bootstrapProxyClient,
				Getter:                  bootstrapProxyClient,
				ClusterProxy:            bootstrapClusterProxy,
				Cluster:                 result.Cluster,
				TTL:                     "1y",
				WaitForRefreshIntervals: e2eConfig.GetIntervals(specName, "wait-machine-refresh"),
			})

			By("Refreshing certificates for the worker nodes")
			ApplyCertificateRefreshForWorker(ctx, ApplyCertificateRefreshForWorkerInput{
				Lister:                  bootstrapProxyClient,
				Getter:                  bootstrapProxyClient,
				ClusterProxy:            bootstrapClusterProxy,
				Cluster:                 result.Cluster,
				MachineDeployments:      result.MachineDeployments,
				TTL:                     "1y",
				WaitForRefreshIntervals: e2eConfig.GetIntervals(specName, "wait-machine-refresh"),
			})

			By("Verifying certificates expiry dates are updated")
			machineList := &clusterv1.MachineList{}
			Expect(bootstrapProxyClient.List(ctx, machineList,
				client.InNamespace(result.Cluster.Namespace),
				client.MatchingLabels{clusterv1.ClusterNameLabel: result.Cluster.Name},
			)).To(Succeed())

			for _, machine := range machineList.Items {
				mAnnotations := machine.GetAnnotations()
				Expect(mAnnotations).To(HaveKey(bootstrapv1.MachineCertificatesExpiryDateAnnotation))
				Expect(mAnnotations[bootstrapv1.CertificatesRefreshStatusAnnotation]).To(Equal(bootstrapv1.CertificatesRefreshDoneStatus))

				_, err := time.Parse(time.RFC3339, mAnnotations[bootstrapv1.MachineCertificatesExpiryDateAnnotation])
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})
})
