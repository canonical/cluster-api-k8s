package apiv1

// SnapRefreshRequest is used to issue a snap refresh.
type SnapRefreshRequest struct {
	// Channel is the channel to refresh the snap to.
	Channel string `json:"channel"`
	// Revision is the revision number to refresh the snap to.
	Revision string `json:"revision"`
	// LocalPath is the local path to use to refresh the snap.
	LocalPath string `json:"localPath"`
}

type SnapRefreshResponse struct {
	ChangeID string `json:"changeId"`
}

type SnapRefreshStatusRequest struct {
	ChangeID string `json:"changeId"`
}

type SnapRefreshStatusResponse struct {
	// Status is the status of the snap refresh/install operation.
	Status string `json:"status"`
	// Completed is a boolean indicating if the snap refresh/install operation has completed.
	// The status should be considered final when this is true.
	Completed bool `json:"completed"`
	// ErrorMessage is the error message if the snap refresh/install operation failed.
	ErrorMessage string `json:"errorMessage"`
}
