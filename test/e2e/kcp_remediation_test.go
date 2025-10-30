//go:build e2e
// +build e2e

/*
Copyright 2020 The Kubernetes Authors.

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
	. "github.com/onsi/ginkgo/v2"
	"k8s.io/utils/ptr"
	capi_e2e "sigs.k8s.io/cluster-api/test/e2e"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
)

var _ = Describe("When testing KCP remediation", func() {
	// See kubernetes.slack.com/archives/C8TSNPY4T/p1680525266510109
	// And github.com/kubernetes-sigs/cluster-api-provider-aws/issues/4198
	if clusterctl.DefaultInfrastructureProvider == "aws" {
		Skip("Skipping KCP remediation test for AWS")
	}

	capi_e2e.KCPRemediationSpec(ctx, func() capi_e2e.KCPRemediationSpecInput {
		return capi_e2e.KCPRemediationSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To(clusterctl.DefaultInfrastructureProvider),
			PostNamespaceCreated: func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string) {
				createLXCSecret(ctx, bootstrapClusterProxy, e2eConfig, workloadClusterNamespace)
			},
		}
	})
})
