package token

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GenerateAndStoreNodeToken(ctx context.Context, ctrlclient client.Client, clusterKey client.ObjectKey, machineName string) (*string, error) {
	tokn, err := randomB64(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate node token: %v", err)
	}

	secret, err := getSecret(ctx, ctrlclient, clusterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup token secret: %v", err)
	}

	patch := client.StrategicMergeFrom(secret, client.MergeFromWithOptimisticLock{})

	newSecret := secret.DeepCopy()
	newSecret.Data[fmt.Sprintf("node-token-%s", machineName)] = []byte(tokn)

	// as secret creation and scope.Config status patch are not atomic operations
	// it is possible that secret creation happens but the config.Status patches are not applied
	if err := ctrlclient.Patch(ctx, newSecret, patch); err != nil {
		return nil, fmt.Errorf("failed to store node token: %v", err)
	}

	return &tokn, nil
}

func LookupNodeToken(ctx context.Context, ctrlclient client.Client, clusterKey client.ObjectKey, machineName string) (*string, error) {
	var s *corev1.Secret
	var err error

	if s, err = getSecret(ctx, ctrlclient, clusterKey); err != nil {
		return nil, fmt.Errorf("failed to lookup token: %v", err)
	}
	if val, ok := s.Data[fmt.Sprintf("node-token-%s", machineName)]; ok {
		ret := string(val)
		return &ret, nil
	}

	return nil, fmt.Errorf("node-token not found")
}
