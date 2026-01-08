package cloudinit_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/canonical/cluster-api-k8s/pkg/cloudinit"
)

func TestMergeBootstrapConfigFileContents(t *testing.T) {
	tests := []struct {
		name               string
		userConfigStr      string
		generatedConfigStr string
		expectedResult     string
		expectError        bool
	}{
		{
			name:          "empty user config returns generated config",
			userConfigStr: "",
			generatedConfigStr: `cluster-config:
  annotations:
    annotation1: value1
    annotation2: value2
  dns:
    enabled: true
  gateway: {}
  ingress:
    enabled: true
  load-balancer: {}
  local-storage: {}
  metrics-server: {}
  network: {}
control-plane-taints:
- taint2
- taint2
`,
			expectedResult: `cluster-config:
  annotations:
    annotation1: value1
    annotation2: value2
  dns:
    enabled: true
  gateway: {}
  ingress:
    enabled: true
  load-balancer: {}
  local-storage: {}
  metrics-server: {}
  network: {}
control-plane-taints:
- taint2
- taint2
`,
		},
		{
			name:               "empty generated config returns user config",
			generatedConfigStr: "",
			userConfigStr: `cluster-config:
  annotations:
    annotation1: value1
    annotation2: value2
  dns:
    enabled: true
  gateway: {}
  ingress:
    enabled: true
  load-balancer: {}
  local-storage: {}
  metrics-server: {}
  network: {}
control-plane-taints:
- taint2
- taint2
`,
			expectedResult: `cluster-config:
  annotations:
    annotation1: value1
    annotation2: value2
  dns:
    enabled: true
  gateway: {}
  ingress:
    enabled: true
  load-balancer: {}
  local-storage: {}
  metrics-server: {}
  network: {}
control-plane-taints:
- taint2
- taint2
`,
		},
		{
			name:          "invalid yaml user config returns error",
			userConfigStr: "#!/bin/bash\necho 'hello world'\n",
			generatedConfigStr: `cluster-config:
  annotations:
    annotation1: value1
    annotation2: value2
  dns:
    enabled: true
  gateway: {}
  ingress:
    enabled: true
  load-balancer: {}
  local-storage: {}
  metrics-server: {}
  network: {}
control-plane-taints:
- taint2
- taint2
`,
			expectError: true,
		},
		{
			name: "invalid generated config returns error",
			userConfigStr: `cluster-config:
  annotations:
    annotation1: value1
    annotation2: value2
  dns:
    enabled: true
  gateway: {}
  ingress:
    enabled: true
  load-balancer: {}
  local-storage: {}
  metrics-server: {}
  network: {}
control-plane-taints:
- taint2
- taint2
`,
			generatedConfigStr: "invalid yaml",
			expectError:        true,
		},
		{
			name: "maps get merged",
			userConfigStr: `cluster-config:
  annotations:
    annotation1: value1
    annotation2: value2
  dns:
    enabled: true
  gateway: {}
  ingress:
    enabled: true
  load-balancer: {}
  local-storage: {}
  metrics-server: {}
  network: {}
control-plane-taints:
- taint2
- taint2`,
			generatedConfigStr: `cluster-config:
  annotations:
    annotation3: value3
  dns:
    enabled: true
  gateway: {}
  ingress:
    enabled: true
  load-balancer: {}
  local-storage: {}
  metrics-server: {}
  network: {}
control-plane-taints:
- taint2
- taint2`,
			expectedResult: `cluster-config:
  annotations:
    annotation1: value1
    annotation2: value2
    annotation3: value3
  dns:
    enabled: true
  gateway: {}
  ingress:
    enabled: true
  load-balancer: {}
  local-storage: {}
  metrics-server: {}
  network: {}
control-plane-taints:
- taint2
- taint2
`,
		},
		{
			name: "slices are replaced",
			userConfigStr: `cluster-config:
  annotations:
    annotation1: value1
    annotation2: value2
  dns:
    enabled: true
  gateway: {}
  ingress:
    enabled: true
  load-balancer: {}
  local-storage: {}
  metrics-server: {}
  network: {}
control-plane-taints:
- taint1
- taint2
`,
			generatedConfigStr: `cluster-config:
  annotations:
    annotation1: value1
    annotation2: value2
  dns:
    enabled: true
  gateway: {}
  ingress:
    enabled: true
  load-balancer: {}
  local-storage: {}
  metrics-server: {}
  network: {}
control-plane-taints:
- taint3
`,
			expectedResult: `cluster-config:
  annotations:
    annotation1: value1
    annotation2: value2
  dns:
    enabled: true
  gateway: {}
  ingress:
    enabled: true
  load-balancer: {}
  local-storage: {}
  metrics-server: {}
  network: {}
control-plane-taints:
- taint1
- taint2
`,
		},
		{
			name: "user config overwrites default",
			generatedConfigStr: `ca-crt: ca-crt
ca-key: ca-key
client-ca-crt: client-ca-crt
client-ca-key: client-ca-key
cluster-config:
  annotations:
    k8sd/v1alpha/lifecycle/skip-cleanup-kubernetes-node-on-remove: "true"
    k8sd/v1alpha/lifecycle/skip-stop-services-on-remove: "true"
  cloud-provider: external
  dns:
    cluster-domain: cluster.local
    enabled: true
  gateway:
    enabled: true
  ingress:
    enabled: true
    service-type: NodePort
  load-balancer:
    enabled: false
  local-storage:
    enabled: true
  metrics-server:
    enabled: true
  network:
    enabled: true
datastore-type: k8s-dqlite
extra-node-kubelet-args:
  --provider-id: openstack:///34a19eef-f39f-44c1-96eb-80e3cd0ec641
extra-sans:
  - 172.16.2.205
k8s-dqlite-port: 2379
pod-cidr: 10.1.0.0/16
service-cidr: 10.152.183.0/24`,
			userConfigStr: `cluster-config:
  annotations:
    k8sd/v1alpha/lifecycle/skip-cleanup-kubernetes-node-on-remove: "true"
  network:
    enabled: true
  dns:
    enabled: true
    cluster-domain: cluster.local
    upstream-nameservers:
    - 8.8.8.8
  local-storage:
    enabled: true
    reclaim-policy: Retain
  metrics-server:
    enabled: false
  load-balancer:
    enabled: true
    l2-mode: false
  ingress:
    enabled: false`,
			expectedResult: `ca-crt: ca-crt
ca-key: ca-key
client-ca-crt: client-ca-crt
client-ca-key: client-ca-key
cluster-config:
  annotations:
    k8sd/v1alpha/lifecycle/skip-cleanup-kubernetes-node-on-remove: "true"
    k8sd/v1alpha/lifecycle/skip-stop-services-on-remove: "true"
  cloud-provider: external
  dns:
    cluster-domain: cluster.local
    enabled: true
    upstream-nameservers:
    - 8.8.8.8
  gateway:
    enabled: true
  ingress:
    enabled: false
  load-balancer:
    enabled: true
    l2-mode: false
  local-storage:
    enabled: true
    reclaim-policy: Retain
  metrics-server:
    enabled: false
  network:
    enabled: true
datastore-type: k8s-dqlite
extra-node-kubelet-args:
  --provider-id: openstack:///34a19eef-f39f-44c1-96eb-80e3cd0ec641
extra-sans:
- 172.16.2.205
k8s-dqlite-port: 2379
pod-cidr: 10.1.0.0/16
service-cidr: 10.152.183.0/24
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := cloudinit.MergeBootstrapConfigFileContents(tt.userConfigStr, tt.generatedConfigStr)
			g := NewWithT(t)
			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result).To(Equal(tt.expectedResult))
			}
		})
	}
}
