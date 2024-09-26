package v1beta2

const (
	CertificatesRefreshAnnotation = "v1beta2.k8sd.io/refresh-certificates"
)

const (
	CertificatesRefreshInProgressEvent = "CertificatesRefreshInProgress"
	CertificatesRefreshDoneEvent       = "CertificatesRefreshDone"
	CertificatesRefreshFailedEvent     = "CertificatesRefreshFailed"
)
