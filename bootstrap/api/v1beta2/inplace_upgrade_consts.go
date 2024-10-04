package v1beta2

const (
	InPlaceUpgradeToAnnotation                  = "v1beta2.k8sd.io/in-place-upgrade-to"
	InPlaceUpgradeStatusAnnotation              = "v1beta2.k8sd.io/in-place-upgrade-status"
	InPlaceUpgradeReleaseAnnotation             = "v1beta2.k8sd.io/in-place-upgrade-release"
	InPlaceUpgradeChangeIDAnnotation            = "v1beta2.k8sd.io/in-place-upgrade-change-id"
	InPlaceUpgradeLastFailedAttemptAtAnnotation = "v1beta2.k8sd.io/in-place-upgrade-last-failed-attempt-at"
)

const (
	InPlaceUpgradeInProgressStatus = "in-progress"
	InPlaceUpgradeDoneStatus       = "done"
	InPlaceUpgradeFailedStatus     = "failed"
)

const (
	InPlaceUpgradeInProgressEvent = "InPlaceUpgradeInProgress"
	InPlaceUpgradeDoneEvent       = "InPlaceUpgradeDone"
	InPlaceUpgradeFailedEvent     = "InPlaceUpgradeFailed"
	InPlaceUpgradeCanceledEvent   = "InPlaceUpgradeCanceled"
)
