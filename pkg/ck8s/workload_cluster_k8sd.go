package ck8s

import (
	"context"
	"crypto/tls"
	_ "embed"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	podv1 "k8s.io/kubernetes/pkg/api/v1/pod"

	ck8serrors "github.com/canonical/cluster-api-k8s/pkg/errors"
	"github.com/canonical/cluster-api-k8s/pkg/proxy"
)

type K8sdClient struct {
	NodeIP string
	Client *http.Client
}

type k8sdClientGenerator struct {
	restConfig         *rest.Config
	clientset          *kubernetes.Clientset
	proxyClientTimeout time.Duration
}

func NewK8sdClientGenerator(restConfig *rest.Config, proxyClientTimeout time.Duration) (*k8sdClientGenerator, error) {
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return &k8sdClientGenerator{
		restConfig:         restConfig,
		clientset:          clientset,
		proxyClientTimeout: proxyClientTimeout,
	}, nil
}

func (g *k8sdClientGenerator) forNode(ctx context.Context, node *corev1.Node) (*K8sdClient, error) {
	podmap, err := g.getProxyPods(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get proxy pods: %w", err)
	}

	pod, ok := podmap[node.Name]
	if !ok {
		return nil, &ck8serrors.K8sdProxyNotFound{NodeName: node.Name}
	}

	if !podv1.IsPodReady(&pod) {
		// if the Pod is not Ready, it won't be able to accept any k8sd API calls.
		return nil, &ck8serrors.K8sdProxyNotReady{PodName: pod.Name}
	}

	return g.forNodePod(ctx, node, pod.Name)
}

func (g *k8sdClientGenerator) forNodePod(ctx context.Context, node *corev1.Node, podname string) (*K8sdClient, error) {
	nodeInternalIP, err := getNodeInternalIP(node)
	if err != nil {
		return nil, fmt.Errorf("failed to get internal IP for node %s: %w", node.Name, err)
	}

	client, err := g.NewHTTPClient(ctx, podname)
	if err != nil {
		return nil, err
	}

	return &K8sdClient{
		NodeIP: nodeInternalIP,
		Client: client,
	}, nil
}

func (g *k8sdClientGenerator) getProxyPods(ctx context.Context) (map[string]corev1.Pod, error) {
	pods, err := g.clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{LabelSelector: "app=k8sd-proxy"})
	if err != nil {
		return nil, fmt.Errorf("unable to list k8sd-proxy pods in target cluster: %w", err)
	}

	podmap := make(map[string]corev1.Pod, len(pods.Items))
	for _, pod := range pods.Items {
		podmap[pod.Spec.NodeName] = pod
	}

	return podmap, nil
}

func (g *k8sdClientGenerator) NewHTTPClient(ctx context.Context, podName string) (*http.Client, error) {
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
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402
		},
		Timeout: g.proxyClientTimeout,
	}, nil
}
