package cloudinit

import (
	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
)

// File is a file that cloud-init will create.
type File struct {
	// Content of the file to create.
	Content string `yaml:"content"`
	// Path where the file should be created.
	Path string `yaml:"path"`
	// Permissions of the file to create, e.g. "0600"
	Permissions string `yaml:"permissions,omitempty"`
	// Owner of the file to create, e.g. "root:root"
	Owner string `yaml:"owner,omitempty"`
	// Encoding is the file encoding, e.g. "base64"
	Encoding string `yaml:"encoding,omitempty"`
}

func FilesFromAPI(files []bootstrapv1.File) []File {
	result := make([]File, 0, len(files))
	for _, file := range files {
		result = append(result, File{
			Content:     file.Content,
			Path:        file.Path,
			Permissions: file.Permissions,
			Owner:       file.Owner,
			Encoding:    string(file.Encoding),
		})
	}

	return result
}
