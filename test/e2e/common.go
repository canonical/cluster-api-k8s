//go:build e2e
// +build e2e

/*
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
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/yaml"
)

// Test suite constants for e2e config variables.
const (
	KubernetesVersionManagement     = "KUBERNETES_VERSION_MANAGEMENT"
	KubernetesVersion               = "KUBERNETES_VERSION"
	KubernetesVersionUpgradeTo      = "KUBERNETES_VERSION_UPGRADE_TO"
	CPMachineTemplateUpgradeTo      = "CONTROL_PLANE_MACHINE_TEMPLATE_UPGRADE_TO"
	WorkersMachineTemplateUpgradeTo = "WORKERS_MACHINE_TEMPLATE_UPGRADE_TO"
	IPFamily                        = "IP_FAMILY"
	InPlaceUpgradeOption            = "IN_PLACE_UPGRADE_OPTION"
)

func Byf(format string, a ...interface{}) {
	By(fmt.Sprintf(format, a...))
}

func setupSpecNamespace(ctx context.Context, specName string, clusterProxy framework.ClusterProxy, artifactFolder string) (*corev1.Namespace, context.CancelFunc) {
	Byf("Creating a namespace for hosting the %q test spec", specName)
	namespace, cancelWatches := framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
		Creator:   clusterProxy.GetClient(),
		ClientSet: clusterProxy.GetClientSet(),
		Name:      fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
		LogFolder: filepath.Join(artifactFolder, "clusters", clusterProxy.GetName()),
	})

	return namespace, cancelWatches
}

type cleanupInput struct {
	SpecName          string
	ClusterProxy      framework.ClusterProxy
	ArtifactFolder    string
	Namespace         *corev1.Namespace
	CancelWatches     context.CancelFunc
	Cluster           *clusterv1.Cluster
	IntervalsGetter   func(spec, key string) []interface{}
	SkipCleanup       bool
	AdditionalCleanup func()
}

func dumpSpecResourcesAndCleanup(ctx context.Context, input cleanupInput) {
	defer func() {
		input.CancelWatches()
	}()

	if input.Cluster == nil {
		By("Unable to dump workload cluster logs as the cluster is nil")
	} else {
		Byf("Dumping logs from the %q workload cluster", input.Cluster.Name)
		input.ClusterProxy.CollectWorkloadClusterLogs(ctx, input.Cluster.Namespace, input.Cluster.Name, filepath.Join(input.ArtifactFolder, "clusters", input.Cluster.Name))
	}

	Byf("Dumping all the Cluster API resources in the %q namespace", input.Namespace.Name)
	// Dump all Cluster API related resources to artifacts before deleting them.
	framework.DumpAllResources(ctx, framework.DumpAllResourcesInput{
		Lister:    input.ClusterProxy.GetClient(),
		Namespace: input.Namespace.Name,
		LogPath:   filepath.Join(input.ArtifactFolder, "clusters", input.ClusterProxy.GetName(), "resources"),
	})

	if input.SkipCleanup {
		return
	}

	Byf("Deleting all clusters in the %s namespace", input.Namespace.Name)
	// While https://github.com/kubernetes-sigs/cluster-api/issues/2955 is addressed in future iterations, there is a chance
	// that cluster variable is not set even if the cluster exists, so we are calling DeleteAllClustersAndWait
	// instead of DeleteClusterAndWait
	framework.DeleteAllClustersAndWait(ctx, framework.DeleteAllClustersAndWaitInput{
		Client:    input.ClusterProxy.GetClient(),
		Namespace: input.Namespace.Name,
	}, input.IntervalsGetter(input.SpecName, "wait-delete-cluster")...)

	Byf("Deleting namespace used for hosting the %q test spec", input.SpecName)
	framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
		Deleter: input.ClusterProxy.GetClient(),
		Name:    input.Namespace.Name,
	})

	if input.AdditionalCleanup != nil {
		Byf("Running additional cleanup for the %q test spec", input.SpecName)
		input.AdditionalCleanup()
	}
}

func localLoadE2EConfig(configPath string) *clusterctl.E2EConfig {
	configData, err := os.ReadFile(configPath) //nolint:gosec
	Expect(err).ToNot(HaveOccurred(), "Failed to read the e2e test config file")
	Expect(configData).ToNot(BeEmpty(), "The e2e test config file should not be empty")

	config := &clusterctl.E2EConfig{}
	Expect(yaml.Unmarshal(configData, config)).To(Succeed(), "Failed to convert the e2e test config file to yaml")

	config.Defaults()
	config.AbsPaths(filepath.Dir(configPath))

	// TODO: this is the reason why we can't use this at present for the RKE2 tests
	// Expect(config.Validate()).To(Succeed(), "The e2e test config file is not valid")

	return config
}

// createLXCSecretForIncus creates the LXC secret for Incus provider if needed
func createLXCSecretForIncus(ctx context.Context, clusterProxy framework.ClusterProxy, e2eConfig *clusterctl.E2EConfig, namespace string) {
	// Check if using incus provider
	hasIncusProvider := false
	for _, provider := range e2eConfig.InfrastructureProviders() {
		if provider == "incus" {
			hasIncusProvider = true
			break
		}
	}

	if !hasIncusProvider {
		return
	}

	By("Creating LXC secret for Incus provider")

	// Get values from environment variables with defaults
	homeDir, err := os.UserHomeDir()
	Expect(err).ToNot(HaveOccurred(), "Failed to get user home directory")

	// Environment variables for LXD configuration
	lxdAddress, err := getLXDDefaultAddress()
	Expect(err).ToNot(HaveOccurred(), "Failed to get LXD default address")

	remote := getEnvWithDefault("LXD_REMOTE", lxdAddress)
	serverCertPath := getEnvWithDefault("LXD_SERVER_CERT", filepath.Join(homeDir, "snap/lxd/common/config/servercerts/local-https.crt"))
	clientCertPath := getEnvWithDefault("LXD_CLIENT_CERT", filepath.Join(homeDir, "snap/lxd/common/config/client.crt"))
	clientKeyPath := getEnvWithDefault("LXD_CLIENT_KEY", filepath.Join(homeDir, "snap/lxd/common/config/client.key"))
	project := getEnvWithDefault("LXD_PROJECT", "default")

	createLXCSecretInNamespace(ctx, clusterProxy, namespace, remote, serverCertPath, clientCertPath, clientKeyPath, project)
}

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getLXDDefaultAddress gets the LXD HTTPS address from lxc config
func getLXDDefaultAddress() (string, error) {
	cmd := exec.Command("lxc", "config", "get", "core.https_address")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get LXD address: %w", err)
	}

	address := strings.TrimSpace(string(output))
	if address == "" {
		return "", fmt.Errorf("LXD core.https_address is empty")
	}

	// Ensure it has https:// prefix
	if !strings.HasPrefix(address, "https://") {
		address = "https://" + address
	}

	return address, nil
}

func createLXCSecretInNamespace(ctx context.Context, clusterProxy framework.ClusterProxy, namespace, remote, serverCertPath, clientCertPath, clientKeyPath, project string) {
	clientset := clusterProxy.GetClientSet()

	// Read certificate files
	serverCrt, err := os.ReadFile(serverCertPath)
	if err != nil {
		fmt.Printf("Warning: Failed to read server certificate from %s: %v\n", serverCertPath, err)
		serverCrt = []byte{}
	}

	clientCrt, err := os.ReadFile(clientCertPath)
	if err != nil {
		fmt.Printf("Warning: Failed to read client certificate from %s: %v\n", clientCertPath, err)
		clientCrt = []byte{}
	}

	clientKey, err := os.ReadFile(clientKeyPath)
	if err != nil {
		fmt.Printf("Warning: Failed to read client key from %s: %v\n", clientKeyPath, err)
		clientKey = []byte{}
	}

	// Create the secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lxc-secret",
			Namespace: namespace,
		},
		StringData: map[string]string{
			"server":     remote,
			"server-crt": string(serverCrt),
			"client-crt": string(clientCrt),
			"client-key": string(clientKey),
			"project":    project,
		},
	}

	_, err = clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		// If secret already exists, update it
		_, err = clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
		Expect(err).ToNot(HaveOccurred(), "Failed to create or update LXC secret")
	}

	fmt.Printf("Created LXC secret with server: %s in namespace: %s\n", remote, namespace)
}

// loadLXDProfileForIncus loads the LXD profile from k8s-snap if Incus provider is used
func loadLXDProfileForIncus(e2eConfig *clusterctl.E2EConfig) {
	// Check if using incus provider
	hasIncusProvider := false
	for _, provider := range e2eConfig.InfrastructureProviders() {
		if provider == "incus" {
			hasIncusProvider = true
			break
		}
	}

	if !hasIncusProvider {
		return
	}

	By("Loading LXD profile for Incus provider")

	// Define the profile URL
	profileURL := "https://raw.githubusercontent.com/canonical/k8s-snap/refs/heads/main/tests/integration/lxd-profile.yaml"
	
	// Fetch the profile content
	resp, err := http.Get(profileURL)
	Expect(err).ToNot(HaveOccurred(), "Failed to fetch LXD profile from URL")
	defer resp.Body.Close()
	
	Expect(resp.StatusCode).To(Equal(http.StatusOK), "Failed to fetch LXD profile: HTTP %d", resp.StatusCode)
	
	profileContent, err := io.ReadAll(resp.Body)
	Expect(err).ToNot(HaveOccurred(), "Failed to read LXD profile content")

	// Create a temporary file to store the profile
	tmpFile, err := os.CreateTemp("", "lxd-profile-*.yaml")
	Expect(err).ToNot(HaveOccurred(), "Failed to create temporary file for LXD profile")
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Write the profile content to the temporary file
	_, err = tmpFile.Write(profileContent)
	Expect(err).ToNot(HaveOccurred(), "Failed to write LXD profile to temporary file")
	
	// Close the file before using it with lxc commands
	tmpFile.Close()

	// Check if profile already exists
	checkCmd := exec.Command("lxc", "profile", "show", "k8s-integration")
	if err := checkCmd.Run(); err == nil {
		By("LXD profile 'k8s-integration' already exists, updating it")
		// Profile exists, edit it
		editCmd := exec.Command("lxc", "profile", "edit", "k8s-integration")
		editCmd.Stdin, err = os.Open(tmpFile.Name())
		Expect(err).ToNot(HaveOccurred(), "Failed to open profile file")
		output, err := editCmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred(), "Failed to update LXD profile: %s", string(output))
	} else {
		By("Creating new LXD profile 'k8s-integration'")
		// Profile doesn't exist, create it
		createCmd := exec.Command("lxc", "profile", "create", "k8s-integration")
		output, err := createCmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred(), "Failed to create LXD profile: %s", string(output))

		// Edit the created profile
		editCmd := exec.Command("lxc", "profile", "edit", "k8s-integration")
		editCmd.Stdin, err = os.Open(tmpFile.Name())
		Expect(err).ToNot(HaveOccurred(), "Failed to open profile file")
		output, err = editCmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred(), "Failed to edit LXD profile: %s", string(output))
	}

	fmt.Println("Successfully loaded LXD profile 'k8s-integration' for Incus provider")
}
