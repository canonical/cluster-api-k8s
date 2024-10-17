package inplace

import (
	"context"

	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MachineGetter is an interface that defines the methods used to get machines.
type MachineGetter interface {
	GetMachinesForCluster(ctx context.Context, cluster client.ObjectKey, filters ...collections.Func) (collections.Machines, error)
}

// Patcher is an interface that knows how to patch an object.
type Patcher interface {
	Patch(ctx context.Context, obj client.Object, opts ...patch.Option) error
}
