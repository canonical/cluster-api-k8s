package ck8s

import (
	apiv1 "github.com/canonical/k8s-snap-api/api/v1"

	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
)

type JoinControlPlaneConfig struct {
	ControlPlaneEndpoint string
	ControlPlaneConfig   bootstrapv1.CK8sControlPlaneConfig

	ExtraKubeProxyArgs  map[string]*string
	ExtraKubeletArgs    map[string]*string
	ExtraContainerdArgs map[string]*string
}

func GenerateJoinControlPlaneConfig(cfg JoinControlPlaneConfig) apiv1.ControlPlaneJoinConfig {
	return apiv1.ControlPlaneJoinConfig{
		ExtraSANS: append(cfg.ControlPlaneConfig.ExtraSANs, cfg.ControlPlaneEndpoint),

		ExtraNodeKubeAPIServerArgs:         cfg.ControlPlaneConfig.ExtraKubeAPIServerArgs,
		ExtraNodeKubeControllerManagerArgs: cfg.ControlPlaneConfig.ExtraKubeControllerManagerArgs,
		ExtraNodeKubeSchedulerArgs:         cfg.ControlPlaneConfig.ExtraKubeSchedulerArgs,

		ExtraNodeKubeProxyArgs:  cfg.ExtraKubeProxyArgs,
		ExtraNodeKubeletArgs:    cfg.ExtraKubeletArgs,
		ExtraNodeContainerdArgs: cfg.ExtraContainerdArgs,
	}
}

type JoinWorkerConfig struct {
	ExtraKubeProxyArgs         map[string]*string
	ExtraKubeletArgs           map[string]*string
	ExtraContainerdArgs        map[string]*string
	ExtraK8sAPIServerProxyArgs map[string]*string
}

func GenerateJoinWorkerConfig(cfg JoinWorkerConfig) apiv1.WorkerJoinConfig {
	return apiv1.WorkerJoinConfig{
		ExtraNodeKubeProxyArgs:         cfg.ExtraKubeProxyArgs,
		ExtraNodeKubeletArgs:           cfg.ExtraKubeletArgs,
		ExtraNodeContainerdArgs:        cfg.ExtraContainerdArgs,
		ExtraNodeK8sAPIServerProxyArgs: cfg.ExtraK8sAPIServerProxyArgs,
	}
}
