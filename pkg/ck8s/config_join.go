package ck8s

import (
	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
	apiv1 "github.com/canonical/cluster-api-k8s/pkg/ck8s/api"
)

type JoinControlPlaneConfig struct {
	ControlPlaneEndpoint string
	ControlPlaneConfig   bootstrapv1.CK8sControlPlaneConfig
}

func GenerateJoinControlPlaneConfig(cfg JoinControlPlaneConfig) (apiv1.ControlPlaneNodeJoinConfig, error) {
	return apiv1.ControlPlaneNodeJoinConfig{
		ExtraSANS: append(cfg.ControlPlaneConfig.ExtraSANs, cfg.ControlPlaneEndpoint),
	}, nil
}

type JoinWorkerConfig struct {
}

func GenerateJoinWorkerConfig(cfg JoinWorkerConfig) apiv1.WorkerNodeJoinConfig {
	return apiv1.WorkerNodeJoinConfig{}
}
