package ck8s

import (
	"context"
	"fmt"
	"time"

	"github.com/canonical/cluster-api-k8s/pkg/token"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ManagementCluster defines all behaviors necessary for something to function as a management cluster.
type ManagementCluster interface {
	client.Reader

	GetMachinesForCluster(ctx context.Context, cluster client.ObjectKey, filters ...collections.Func) (collections.Machines, error)
	GetWorkloadCluster(ctx context.Context, clusterKey client.ObjectKey, microclusterPort int) (*Workload, error)
}

// Management holds operations on the management cluster.
type Management struct {
	ManagementCluster

	Client client.Client

	K8sdDialTimeout time.Duration
}

// RemoteClusterConnectionError represents a failure to connect to a remote cluster.
type RemoteClusterConnectionError struct {
	Name string
	Err  error
}

func (e *RemoteClusterConnectionError) Error() string { return e.Name + ": " + e.Err.Error() }
func (e *RemoteClusterConnectionError) Unwrap() error { return e.Err }

// Get implements ctrlclient.Reader.
func (m *Management) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return m.Client.Get(ctx, key, obj)
}

// List implements ctrlclient.Reader.
func (m *Management) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return m.Client.List(ctx, list, opts...)
}

// GetMachinesForCluster returns a list of machines that can be filtered or not.
// If no filter is supplied then all machines associated with the target cluster are returned.
func (m *Management) GetMachinesForCluster(ctx context.Context, cluster client.ObjectKey, filters ...collections.Func) (collections.Machines, error) {
	selector := map[string]string{
		clusterv1.ClusterNameLabel: cluster.Name,
	}
	ml := &clusterv1.MachineList{}
	if err := m.Client.List(ctx, ml, client.InNamespace(cluster.Namespace), client.MatchingLabels(selector)); err != nil {
		return nil, fmt.Errorf("failed to list machines: %w", err)
	}

	machines := collections.FromMachineList(ml)
	return machines.Filter(filters...), nil
}

const (
	// CK8sControlPlaneControllerName defines the controller used when creating clients.
	CK8sControlPlaneControllerName = "ck8s-controlplane-controller"
)

// GetWorkloadCluster builds a cluster object.
// The cluster comes with an etcd client generator to connect to any etcd pod living on a managed machine.
func (m *Management) GetWorkloadCluster(ctx context.Context, clusterKey client.ObjectKey, microclusterPort int) (*Workload, error) {
	restConfig, err := remote.RESTConfig(ctx, CK8sControlPlaneControllerName, m.Client, clusterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}
	restConfig.Timeout = 30 * time.Second

	c, err := client.New(restConfig, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, &RemoteClusterConnectionError{Name: clusterKey.String(), Err: err}
	}

	g, err := NewK8sdClientGenerator(restConfig, m.K8sdDialTimeout)
	if err != nil {
		return nil, &RemoteClusterConnectionError{Name: clusterKey.String(), Err: err}
	}

	authToken, err := token.Lookup(ctx, m.Client, clusterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup auth token: %w", err)
	}

	if authToken == nil {
		return nil, fmt.Errorf("auth token not yet generated")
	}

	workload := &Workload{
		authToken:           *authToken,
		Client:              c,
		ClientRestConfig:    restConfig,
		K8sdClientGenerator: g,
		microclusterPort:    microclusterPort,

		/**
		CoreDNSMigrator: &CoreDNSMigrator{},
		**/
	}
	// NOTE(neoaggelos): Upstream creates an etcd client generator, so that users can reach etcd on each node.
	//
	// TODO(neoaggelos): For Canonical Kubernetes, we need to create a client generator for the k8sd endpoints on the control plane nodes.

	/**
	// Retrieves the etcd CA key Pair
	crtData, keyData, err := m.getEtcdCAKeyPair(ctx, clusterKey)
	if err != nil {
		return nil, err
	}

	// If etcd CA is not nil, then it's managed etcd
	if crtData != nil {
		clientCert, err := generateClientCert(crtData, keyData)
		if err != nil {
			return nil, err
		}

		caPool := x509.NewCertPool()
		caPool.AppendCertsFromPEM(crtData)
		tlsConfig := &tls.Config{
			RootCAs:      caPool,
			Certificates: []tls.Certificate{clientCert},
			MinVersion:   tls.VersionTLS12,
		}
		tlsConfig.InsecureSkipVerify = true
		workload.etcdClientGenerator = NewEtcdClientGenerator(restConfig, tlsConfig, m.EtcdDialTimeout, m.EtcdCallTimeout)
	}
	**/

	return workload, nil
}

//nolint:godot
/**
func (m *Management) getEtcdCAKeyPair(ctx context.Context, clusterKey client.ObjectKey) ([]byte, []byte, error) {
	etcdCASecret := &corev1.Secret{}
	etcdCAObjectKey := client.ObjectKey{
		Namespace: clusterKey.Namespace,
		Name:      fmt.Sprintf("%s-etcd", clusterKey.Name),
	}

	// Try to get the certificate via the uncached client.
	if err := m.Client.Get(ctx, etcdCAObjectKey, etcdCASecret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil, nil
		} else {
			return nil, nil, errors.Wrapf(err, "failed to get secret; etcd CA bundle %s/%s", etcdCAObjectKey.Namespace, etcdCAObjectKey.Name)
		}
	}

	crtData, ok := etcdCASecret.Data[secret.TLSCrtDataName]
	if !ok {
		return nil, nil, errors.Errorf("etcd tls crt does not exist for cluster %s/%s", clusterKey.Namespace, clusterKey.Name)
	}
	keyData := etcdCASecret.Data[secret.TLSKeyDataName]
	return crtData, keyData, nil
}
**/

var _ ManagementCluster = &Management{}
