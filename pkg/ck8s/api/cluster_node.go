package apiv1

// RemoveNodeRequest is used to request to remove a node from the cluster.
type RemoveNodeRequest struct {
	Name  string `json:"name"`
	Force bool   `json:"force"`
}
