package ck8s

import (
	"context"
)

// NewControlPlaneJoinToken creates a new join token for a control plane node.
// NewControlPlaneJoinToken reaches out to the control-plane of the workload cluster via k8sd-proxy client.
func NewControlPlaneJoinToken(ctx context.Context, k8sdProxy *K8sdProxy, authToken string, microclusterPort int, name string) (string, error) {
	return RequestJoinToken(ctx, k8sdProxy.Client, k8sdProxy.NodeIP, microclusterPort, authToken, name, false)
}
