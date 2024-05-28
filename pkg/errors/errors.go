package errors

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
