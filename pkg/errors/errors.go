package errors

import (
	"errors"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

type CK8sControlPlaneStatusError string

const (
	// InvalidConfigurationCK8sControlPlaneError indicates that the CK8s control plane
	// configuration is invalid.
	InvalidConfigurationCK8sControlPlaneError CK8sControlPlaneStatusError = "InvalidConfiguration"

	// UnsupportedChangeCK8sControlPlaneError indicates that the CK8s control plane
	// spec has been updated in an unsupported way that cannot be
	// reconciled.
	UnsupportedChangeCK8sControlPlaneError CK8sControlPlaneStatusError = "UnsupportedChange"

	// CreateCK8sControlPlaneError indicates that an error was encountered
	// when trying to create the CK8s control plane.
	CreateCK8sControlPlaneError CK8sControlPlaneStatusError = "CreateError"

	// UpdateCK8sControlPlaneError indicates that an error was encountered
	// when trying to update the CK8s control plane.
	UpdateCK8sControlPlaneError CK8sControlPlaneStatusError = "UpdateError"

	// DeleteCK8sControlPlaneError indicates that an error was encountered
	// when trying to delete the CK8s control plane.
	DeleteCK8sControlPlaneError CK8sControlPlaneStatusError = "DeleteError"
)

type K8sdProxyNotFound struct {
	NodeName string
}

func (e *K8sdProxyNotFound) Error() string {
	if e.NodeName == "" {
		return "missing k8sd proxy pod(s)"
	}
	return fmt.Sprintf("missing k8sd proxy pod for node %s", e.NodeName)
}

type K8sdProxyNotReady struct {
	PodName string
}

func (e *K8sdProxyNotReady) Error() string {
	return fmt.Sprintf("pod '%s' is not Ready", e.PodName)
}

func RequeueOnK8sdProxyError(err error) (ctrl.Result, error) {
	var (
		notFoundErr *K8sdProxyNotFound
		notReadyErr *K8sdProxyNotReady
	)
	if errors.As(err, &notFoundErr) || errors.As(err, &notReadyErr) {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Not a k8sd-proxy related error.
	return ctrl.Result{}, err
}
