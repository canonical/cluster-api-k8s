package ck8s

import (
	apiv1 "github.com/canonical/k8s-snap-api/api/v1"

	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
)

type JoinControlPlaneConfig struct {
	ControlPlaneEndpoint string
	ControlPlaneConfig   bootstrapv1.CK8sControlPlaneConfig

	ExtraKubeAPIServerArgs map[string]*string
}

func GenerateJoinControlPlaneConfig(cfg JoinControlPlaneConfig) apiv1.ControlPlaneJoinConfig {
	return apiv1.ControlPlaneJoinConfig{
		ExtraSANS: append(cfg.ControlPlaneConfig.ExtraSANs, cfg.ControlPlaneEndpoint),

		ExtraNodeKubeAPIServerArgs: cfg.ControlPlaneConfig.ExtraKubeAPIServerArgs,
	}
}

type JoinWorkerConfig struct {
}

func GenerateJoinWorkerConfig(cfg JoinWorkerConfig) apiv1.WorkerJoinConfig {
	return apiv1.WorkerJoinConfig{}
}
