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
