package ck8s

import (
	"context"
	"crypto/tls"
	_ "embed"
	"fmt"
	"net/http"
	"time"

	"github.com/canonical/cluster-api-k8s/pkg/proxy"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const K8sdProxyDaemonsetYamlLocation = "/opt/capi/manifests/k8sd-proxy.yaml"

//go:embed k8sd-proxy.yaml
var K8sdProxyDaemonsetYaml string

type K8sdProxy struct {
	nodeIP string
	client *http.Client
}

type K8sdProxyGenerator struct {
	restConfig         *rest.Config
	clientset          *kubernetes.Clientset
	proxyClientTimeout time.Duration
	k8sdPort           int
}

func NewK8sdProxyGenerator(restConfig *rest.Config, clientset *kubernetes.Clientset, proxyClientTimeout time.Duration, k8sdPort int) (*K8sdProxyGenerator, error) {
	return &K8sdProxyGenerator{
		restConfig:         restConfig,
		clientset:          clientset,
		proxyClientTimeout: proxyClientTimeout,
		k8sdPort:           k8sdPort,
	}, nil
}

func (g *K8sdProxyGenerator) forNodeName(ctx context.Context, nodeName string) (*K8sdProxy, error) {
	node, err := g.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "unable to get node in target cluster")
	}

	return g.forNode(ctx, node)
}

func (g *K8sdProxyGenerator) forNode(ctx context.Context, node *corev1.Node) (*K8sdProxy, error) {
	podmap, err := g.getProxyPods(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get proxy pods: %w", err)
	}

	podname, ok := podmap[node.Name]
	if !ok {
		return nil, fmt.Errorf("this node does not have a k8sd proxy pod")
	}

	nodeInternalIP, err := g.getNodeInternalIP(node)
	if err != nil {
		return nil, fmt.Errorf("failed to get internal IP for node %s: %w", node.Name, err)
	}

	client, err := g.NewHTTPClient(ctx, podname)
	if err != nil {
		return nil, err
	}

	if err := g.checkIfK8sdIsReachable(ctx, client, nodeInternalIP); err != nil {
		return nil, fmt.Errorf("failed to reach k8sd through proxy client: %w", err)
	}

	return &K8sdProxy{
		nodeIP: nodeInternalIP,
		client: client,
	}, nil
}

func (g *K8sdProxyGenerator) forControlPlane(ctx context.Context) (*K8sdProxy, error) {
	cplaneNodes, err := g.getControlPlaneNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get control plane nodes: %w", err)
	}

	for _, node := range cplaneNodes.Items {
		proxy, err := g.forNode(ctx, &node)
		if err != nil {
			continue
		}

		return proxy, nil
	}

	return nil, fmt.Errorf("failed to find a control plane node with a reachable k8sd proxy")
}

func (g *K8sdProxyGenerator) checkIfK8sdIsReachable(ctx context.Context, client *http.Client, nodeIP string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s:%v/", nodeIP, g.k8sdPort), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach k8sd through proxy client: %w", err)
	}
	res.Body.Close()

	return nil
}

func (g *K8sdProxyGenerator) getNodeInternalIP(node *corev1.Node) (string, error) {
	// TODO: Make this more robust by possibly finding/parsing the right IP.
	// This works as a start but might not be sufficient as the kubelet IP might not match microcluster IP.
	for _, addr := range node.Status.Addresses {
		if addr.Type == "InternalIP" {
			return addr.Address, nil
		}
	}

	return "", fmt.Errorf("unable to find internal IP for node %s", node.Name)
}

func (g *K8sdProxyGenerator) getControlPlaneNodes(ctx context.Context) (*corev1.NodeList, error) {
	nodes, err := g.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: "node-role.kubernetes.io/control-plane="})
	if err != nil {
		return nil, errors.Wrap(err, "unable to list nodes in target cluster")
	}

	if len(nodes.Items) == 0 {
		return nil, errors.New("there isn't any nodes registered in target cluster")
	}

	return nodes, nil
}

func (g *K8sdProxyGenerator) getProxyPods(ctx context.Context) (map[string]string, error) {
	pods, err := g.clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{LabelSelector: "app=k8sd-proxy"})
	if err != nil {
		return nil, errors.Wrap(err, "unable to list k8sd-proxy pods in target cluster")
	}

	if len(pods.Items) == 0 {
		return nil, errors.New("there isn't any k8sd-proxy pods in target cluster")
	}

	podmap := make(map[string]string, len(pods.Items))
	for _, pod := range pods.Items {
		podmap[pod.Spec.NodeName] = pod.Name
	}

	return podmap, nil
}

func (g *K8sdProxyGenerator) NewHTTPClient(ctx context.Context, podName string) (*http.Client, error) {
	p := proxy.Proxy{
		Kind:         "pods",
		Namespace:    metav1.NamespaceSystem,
		ResourceName: podName,
		KubeConfig:   g.restConfig,
		Port:         2380,
	}

	dialer, err := proxy.NewDialer(p)
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy dialer: %w", err)
	}

	// We return a http client with the same parameters as http.DefaultClient
	// and an overridden DialContext to proxy the requests through api server.
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.DefaultTransport.(*http.Transport).Proxy,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     http.DefaultTransport.(*http.Transport).ForceAttemptHTTP2,
			MaxIdleConns:          http.DefaultTransport.(*http.Transport).MaxIdleConns,
			IdleConnTimeout:       http.DefaultTransport.(*http.Transport).IdleConnTimeout,
			TLSHandshakeTimeout:   http.DefaultTransport.(*http.Transport).TLSHandshakeTimeout,
			ExpectContinueTimeout: http.DefaultTransport.(*http.Transport).ExpectContinueTimeout,
			// TODO: Workaround for now, address later on
			// get the certificate fingerprint from the matching node through a resource in the cluster (TBD), and validate it in the TLSClientConfig
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: g.proxyClientTimeout,
	}, nil
}
