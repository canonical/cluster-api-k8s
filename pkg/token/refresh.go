package token

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func EnsureNodeToken(ctx context.Context, ctrlclient client.Client, clusterKey client.ObjectKey, machineName string) (*string, error) {
	logger := log.FromContext(ctx).WithValues("machine", machineName, "func", "EnsureNodeToken")

	var (
		token string
		err   error
	)

	token, err = LookupNodeToken(ctx, ctrlclient, clusterKey, machineName)
	if err == nil {
		logger.Info("Node token already exists")
		return &token, nil
	}

	logger.Info("Node token not found, generating a new one")

	token, err = randomB64(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate node token: %v", err)
	}

	secret, err := getSecret(ctx, ctrlclient, clusterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup token secret: %v", err)
	}

	patch := client.StrategicMergeFrom(secret, client.MergeFromWithOptimisticLock{})

	newSecret := secret.DeepCopy()
	newSecret.Data[machineNodeTokenEntry(machineName)] = []byte(token)

	// as secret creation and scope.Config status patch are not atomic operations
	// it is possible that secret creation happens but the config.Status patches are not applied
	if err := ctrlclient.Patch(ctx, newSecret, patch); err != nil {
		return nil, fmt.Errorf("failed to patch token secret: %v", err)
	}

	logger.Info("Stored the new node token in the secret")

	return &token, nil
}

func LookupNodeToken(ctx context.Context, ctrlclient client.Client, clusterKey client.ObjectKey, machineName string) (string, error) {
	s, err := getSecret(ctx, ctrlclient, clusterKey)
	if err != nil {
		return "", fmt.Errorf("failed to get token secret: %v", err)
	}

	if val, ok := s.Data[machineNodeTokenEntry(machineName)]; ok {
		return string(val), nil
	}

	return "", fmt.Errorf("node-token for machine %q not found", machineName)
}
