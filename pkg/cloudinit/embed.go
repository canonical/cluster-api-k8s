package cloudinit

import (
	"embed"
	"path"
)

var (
	//go:embed scripts
	embeddedScriptsFS embed.FS
)

type script string

var (
	scriptInstall            script = "install.sh"
	scriptBootstrap          script = "bootstrap.sh"
	scriptConfigureToken     script = "configure-token.sh"
	scriptWaitAPIServerReady script = "wait-apiserver-ready.sh"
	scriptDeployManifests    script = "deploy-manifests.sh"
)

func mustEmbed(s script) string {
	b, err := embeddedScriptsFS.ReadFile(path.Join("scripts", string(s)))
	if err != nil {
		panic(err)
	}
	return string(b)
}

var (
	// scripts is a map of all embedded bash scripts used in the cloud-init.
	scripts = map[script]string{
		scriptInstall:            mustEmbed(scriptInstall),
		scriptBootstrap:          mustEmbed(scriptBootstrap),
		scriptConfigureToken:     mustEmbed(scriptConfigureToken),
		scriptWaitAPIServerReady: mustEmbed(scriptWaitAPIServerReady),
		scriptDeployManifests:    mustEmbed(scriptDeployManifests),
	}
)
