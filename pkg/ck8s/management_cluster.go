package ck8s

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/canonical/cluster-api-k8s/pkg/token"
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

	drainer := NewDrainer(c, time.Now, DrainOptions{
		Force:                 true,
		AllowDeletion:         true,
		IgnoreDaemonsets:      true,
		DeleteEmptydirData:    true,
		Timeout:               5 * time.Minute,
		EvictionTimeout:       1 * time.Minute,
		EvictionRetryInterval: 20 * time.Second,
		GracePeriodSeconds:    10,
	})

	workload := &Workload{
		authToken:           *authToken,
		Client:              c,
		ClientRestConfig:    restConfig,
		K8sdClientGenerator: g,
		microclusterPort:    microclusterPort,
		drainer:             drainer,
	}

	return workload, nil
}

var _ ManagementCluster = &Management{}
