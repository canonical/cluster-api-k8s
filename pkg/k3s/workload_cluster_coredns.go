/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package k3s

import (
	"context"
	"errors"
	"fmt"

	"github.com/coredns/corefile-migration/migration"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/version"

	controlplanev1 "github.com/k3s-io/cluster-api-k3s/controlplane/api/v1beta2"
)

const (
	corefileKey       = "Corefile"
	corefileBackupKey = "Corefile-backup"
	coreDNSKey        = "coredns"
	coreDNSVolumeKey  = "config-volume"
)

type coreDNSMigrator interface {
	Migrate(currentVersion string, toVersion string, corefile string, deprecations bool) (string, error)
}

type CoreDNSMigrator struct{}

func (c *CoreDNSMigrator) Migrate(fromCoreDNSVersion, toCoreDNSVersion, corefile string, deprecations bool) (string, error) {
	return migration.Migrate(fromCoreDNSVersion, toCoreDNSVersion, corefile, deprecations)
}

type coreDNSInfo struct {
	Corefile   string
	Deployment *appsv1.Deployment

	FromImageTag string
	ToImageTag   string

	CurrentMajorMinorPatch string
	TargetMajorMinorPatch  string

	FromImage string
	ToImage   string
}

var ErrInvalidCoreDNSVersion = errors.New("invalid CoreDNS version given")

// UpdateCoreDNS updates the coredns corefile and coredns
// deployment.
func (w *Workload) UpdateCoreDNS(ctx context.Context, kcp *controlplanev1.KThreesControlPlane) error {
	// Return early if we've been asked to skip CoreDNS upgrades entirely.
	if _, ok := kcp.Annotations[controlplanev1.SkipCoreDNSAnnotation]; ok {
		return nil
	}
	/*
		// Return early if the configuration is nil.
		if kcp.Spec.KThreesConfigSpec.ServerConfig == KThreesServerConfig {
			return nil
		}

		clusterConfig := kcp.Spec.KThreesConfigSpec.ServerConfig

		// Return early if the type is anything other than empty (default), or CoreDNS.
		if clusterConfig.DNS.Type != "" && clusterConfig.DNS.Type != kubeadmv1.CoreDNS {
			return nil
		}

		// Get the CoreDNS info needed for the upgrade.
		info, err := w.getCoreDNSInfo(ctx, clusterConfig)
		if err != nil {
			// Return early if we get a not found error, this can happen if any of the CoreDNS components
			// cannot be found, e.g. configmap, deployment.
			if apierrors.IsNotFound(errors.Cause(err)) {
				return nil
			}
			return err
		}

		// Return early if the from/to image is the same.
		if info.FromImage == info.ToImage {
			return nil
		}

		// Validate the image tag.
		if err := validateCoreDNSImageTag(info.FromImageTag, info.ToImageTag); err != nil {
			return fmt.Errorf(err, "failed to validate CoreDNS")
		}

		// Perform the upgrade.
		if err := w.updateCoreDNSImageInfoInKubeadmConfigMap(ctx, &clusterConfig.DNS); err != nil {
			return err
		}
		if err := w.updateCoreDNSCorefile(ctx, info); err != nil {
			return err
		}
		if err := w.updateCoreDNSDeployment(ctx, info); err != nil {
			return fmt.Errorf("unable to update coredns deployment")
		}
	*/
	return nil
}

// func (w *Workload) getConfigMap(ctx context.Context, configMap ctrlclient.ObjectKey) (*corev1.ConfigMap, error) {
// 	original := &corev1.ConfigMap{}
// 	if err := w.Client.Get(ctx, configMap, original); err != nil {
// 		return nil, fmt.Errorf(err, "error getting %s/%s configmap from target cluster", configMap.Namespace, configMap.Name)
// 	}
// 	return original.DeepCopy(), nil
// }.

// // getCoreDNSInfo returns all necessary coredns based information.
// func (w *Workload) getCoreDNSInfo(ctx context.Context, clusterConfig *kubeadmv1.ClusterConfiguration) (*coreDNSInfo, error) {
// 	// Get the coredns configmap and corefile.
// 	key := ctrlclient.ObjectKey{Name: coreDNSKey, Namespace: metav1.NamespaceSystem}
// 	cm, err := w.getConfigMap(ctx, key)
// 	if err != nil {
// 		return nil, fmt.Errorf(err, "error getting %v config map from target cluster", key)
// 	}
// 	corefile, ok := cm.Data[corefileKey]
// 	if !ok {
// 		return nil, errors.New("unable to find the CoreDNS Corefile data")
// 	}

// 	// Get the current CoreDNS deployment.
// 	deployment := &appsv1.Deployment{}
// 	if err := w.Client.Get(ctx, key, deployment); err != nil {
// 		return nil, fmt.Errorf(err, "unable to get %v deployment from target cluster", key)
// 	}

// 	var container *corev1.Container
// 	for _, c := range deployment.Spec.Template.Spec.Containers {
// 		if c.Name == coreDNSKey {
// 			container = c.DeepCopy()
// 			break
// 		}
// 	}
// 	if container == nil {
// 		return nil, errors.Errorf("failed to update coredns deployment: deployment spec has no %q container", coreDNSKey)
// 	}

// 	// Parse container image.
// 	parsedImage, err := containerutil.ImageFromString(container.Image)
// 	if err != nil {
// 		return nil, fmt.Errorf(err, "unable to parse %q deployment image", container.Image)
// 	}

// 	// Handle imageRepository.
// 	toImageRepository := fmt.Sprintf("%s/%s", parsedImage.Repository, parsedImage.Name)
// 	if clusterConfig.ImageRepository != "" {
// 		toImageRepository = fmt.Sprintf("%s/%s", clusterConfig.ImageRepository, coreDNSKey)
// 	}
// 	if clusterConfig.DNS.ImageRepository != "" {
// 		toImageRepository = fmt.Sprintf("%s/%s", clusterConfig.DNS.ImageRepository, coreDNSKey)
// 	}

// 	// Handle imageTag.
// 	if parsedImage.Tag == "" {
// 		return nil, errors.Errorf("failed to update coredns deployment: does not have a valid image tag: %q", container.Image)
// 	}
// 	currentMajorMinorPatch, err := extractImageVersion(parsedImage.Tag)
// 	if err != nil {
// 		return nil, err
// 	}
// 	toImageTag := parsedImage.Tag
// 	if clusterConfig.DNS.ImageTag != "" {
// 		toImageTag = clusterConfig.DNS.ImageTag
// 	}
// 	targetMajorMinorPatch, err := extractImageVersion(toImageTag)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return &coreDNSInfo{
// 		Corefile:               corefile,
// 		Deployment:             deployment,
// 		CurrentMajorMinorPatch: currentMajorMinorPatch,
// 		TargetMajorMinorPatch:  targetMajorMinorPatch,
// 		FromImageTag:           parsedImage.Tag,
// 		ToImageTag:             toImageTag,
// 		FromImage:              container.Image,
// 		ToImage:                fmt.Sprintf("%s:%s", toImageRepository, toImageTag),
// 	}, nil
// }.

// UpdateCoreDNSDeployment will patch the deployment image to the
// imageRepo:imageTag in the KCP dns. It will also ensure the volume of the
// deployment uses the Corefile key of the coredns configmap.
// func (w *Workload) updateCoreDNSDeployment(ctx context.Context, info *coreDNSInfo) error {
// 	helper, err := patch.NewHelper(info.Deployment, w.Client)
// 	if err != nil {
// 		return err
// 	}
// 	// Form the final image before issuing the patch.
// 	patchCoreDNSDeploymentImage(info.Deployment, info.ToImage)
// 	// Flip the deployment volume back to Corefile (from the backup key).
// 	patchCoreDNSDeploymentVolume(info.Deployment, corefileBackupKey, corefileKey)
// 	return helper.Patch(ctx, info.Deployment)
// }.

// UpdateCoreDNSImageInfoInKubeadmConfigMap updates the kubernetes version in the kubeadm config map.
// func (w *Workload) updateCoreDNSImageInfoInKubeadmConfigMap(ctx context.Context, dns *kubeadmv1.DNS) error {
// 	return nil
// }.

// updateCoreDNSCorefile migrates the coredns corefile if there is an increase
// in version number. It also creates a corefile backup and patches the
// deployment to point to the backup corefile before migrating.
//
//lint:ignore U1000 Ignore
func (w *Workload) updateCoreDNSCorefile(ctx context.Context, info *coreDNSInfo) error {
	// Run the CoreDNS migration tool first because if it cannot migrate the
	// corefile, then there's no point in continuing further.
	updatedCorefile, err := w.CoreDNSMigrator.Migrate(info.CurrentMajorMinorPatch, info.TargetMajorMinorPatch, info.Corefile, false)
	if err != nil {
		return fmt.Errorf("unable to migrate CoreDNS corefile: %w", err)
	}

	// First we backup the Corefile by backing it up.
	if err := w.Client.Update(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			corefileKey:       info.Corefile,
			corefileBackupKey: info.Corefile,
		},
	}); err != nil {
		return fmt.Errorf("unable to update CoreDNS config map with backup Corefile: %w", err)
	}

	// Patching the coredns deployment to point to the Corefile-backup
	// contents before performing the migration.
	helper, err := patch.NewHelper(info.Deployment, w.Client)
	if err != nil {
		return err
	}
	patchCoreDNSDeploymentVolume(info.Deployment, corefileKey, corefileBackupKey)
	if err := helper.Patch(ctx, info.Deployment); err != nil {
		return err
	}

	if err := w.Client.Update(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			corefileKey:       updatedCorefile,
			corefileBackupKey: info.Corefile,
		},
	}); err != nil {
		return fmt.Errorf("unable to update CoreDNS config map: %w", ErrInvalidCoreDNSVersion)
	}

	return nil
}

//lint:ignore U1000 Ignore
func patchCoreDNSDeploymentVolume(deployment *appsv1.Deployment, fromKey, toKey string) {
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == coreDNSVolumeKey && volume.ConfigMap != nil && volume.ConfigMap.Name == coreDNSKey {
			for i, item := range volume.ConfigMap.Items {
				if item.Key == fromKey || item.Key == toKey {
					volume.ConfigMap.Items[i].Key = toKey
				}
			}
		}
	}
}

//lint:ignore U1000 Ignore
func patchCoreDNSDeploymentImage(deployment *appsv1.Deployment, image string) {
	containers := deployment.Spec.Template.Spec.Containers
	for idx, c := range containers {
		if c.Name == coreDNSKey {
			containers[idx].Image = image
		}
	}
}

//lint:ignore U1000 Ignore
func extractImageVersion(tag string) (string, error) {
	ver, err := version.ParseMajorMinorPatch(tag)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%d.%d", ver.Major, ver.Minor, ver.Patch), nil
}

// validateCoreDNSImageTag returns error if the versions don't meet requirements.
// Some of the checks come from
// https://github.com/coredns/corefile-migration/blob/v1.0.6/migration/migrate.go#L414
// func validateCoreDNSImageTag(fromTag, toTag string) error {
// 	from, err := version.ParseMajorMinorPatch(fromTag)
// 	if err != nil {
// 		return fmt.Errorf("failed to parse CoreDNS current version %q: %w", fromTag, err)
// 	}
// 	to, err := version.ParseMajorMinorPatch(toTag)
// 	if err != nil {
// 		return fmt.Errorf("failed to parse CoreDNS target version %q: %w", toTag, err)
// 	}
// 	// make sure that the version we're upgrading to is greater than the current one,
// 	// or if they're the same version, the raw tags should be different (e.g. allow from `v1.17.4-somevendor.0` to `v1.17.4-somevendor.1`).
// 	if x := from.Compare(to); x > 0 || (x == 0 && fromTag == toTag) {
// 		return fmt.Errorf("toVersion %q must be greater than fromVersion %q: %w", toTag, fromTag, ErrInvalidCoreDNSVersion)
// 	}

// 	// check if the from version is even in the list of coredns versions
// 	if _, ok := migration.Versions[fmt.Sprintf("%d.%d.%d", from.Major, from.Minor, from.Patch)]; !ok {
// 		return fmt.Errorf("fromVersion %q is not a compatible version: %w", fromTag, ErrInvalidCoreDNSVersion)
// 	}
// 	return nil
// }.
