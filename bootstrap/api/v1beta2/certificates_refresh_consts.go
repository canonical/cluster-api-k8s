package v1beta2

const (
	CertificatesRefreshAnnotation       = "v1beta2.k8sd.io/refresh-certificates"
	CertificatesRefreshStatusAnnotation = "v1beta2.k8sd.io/refresh-certificates-status"
)

const (
	CertificatesRefreshInProgressStatus = "in-progress"
	CertificatesRefreshDoneStatus       = "done"
	CertificatesRefreshFailedStatus     = "failed"
)

const (
	CertificatesRefreshInProgressEvent = "CertificatesRefreshInProgress"
	CertificatesRefreshDoneEvent       = "CertificatesRefreshDone"
	CertificatesRefreshFailedEvent     = "CertificatesRefreshFailed"
)
