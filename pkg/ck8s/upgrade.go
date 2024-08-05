package ck8s

const (
	InPlaceUpgradeToAnnotation      = "k8sd.io/in-place-upgrade-to"
	InPlaceUpgradeStatusAnnotation  = "k8sd.io/in-place-upgrade-status"
	InPlaceUpgradeReleaseAnnotation = "k8sd.io/in-place-upgrade-release"
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
)
