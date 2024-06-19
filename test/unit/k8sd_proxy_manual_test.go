package unit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/canonical/cluster-api-k8s/pkg/ck8s"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestK8sdProxyManual(t *testing.T) {
	kubeconfig := os.Getenv("K8SD_PROXY_TEST_KUBECONFIG")
	if kubeconfig == "" {
		t.Skip()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var microclusterPort int = 2380

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("failed to create rest config from kubeconfig: %v", err)
	}
	config.Timeout = 30 * time.Second

	c, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	g, err := ck8s.NewK8sdClientGenerator(config, 10*time.Second)
	if err != nil {
		t.Fatalf("failed to create k8sd proxy generator: %v", err)
	}

	w := &ck8s.Workload{
		Client:              c,
		ClientRestConfig:    config,
		K8sdClientGenerator: g,
	}

	proxy, err := w.GetK8sdProxyForControlPlane(ctx)
	if err != nil {
		t.Fatalf("failed to get k8sd proxy: %v", err)
	}

	if proxy != nil {
		err = ck8s.CheckIfK8sdIsReachable(ctx, proxy.Client, proxy.NodeIP, microclusterPort)
		if err != nil {
			t.Fatalf("k8sd is not reachable: %v", err)
		}
		t.Logf("k8sd is reachable")
	}

}
