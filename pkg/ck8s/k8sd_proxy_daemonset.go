package ck8s

import (
	"bytes"
	_ "embed"
	"text/template"
)

var (
	//go:embed manifests/k8sd-proxy-template.yaml
	k8sdProxyDaemonSetYaml string

	k8sdProxyDaemonSetTemplate *template.Template = template.Must(template.New("K8sdProxyDaemonset").Parse(k8sdProxyDaemonSetYaml))
)

type K8sdProxyDaemonSetInput struct {
	K8sdPort int
}

// RenderK8sdProxyDaemonSet renders the manifest for the k8sd-proxy daemonset based on supplied configuration.
func RenderK8sdProxyDaemonSetManifest(input K8sdProxyDaemonSetInput) ([]byte, error) {
	var b bytes.Buffer
	if err := k8sdProxyDaemonSetTemplate.Execute(&b, input); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}
