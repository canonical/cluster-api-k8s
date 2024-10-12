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

// NOTE(eac): If you want to use a script from pkg/cloudinit/scripts in your code (for example, you want to include a script in the user-data.txt),
// you need to add it to the scripts map below.
var (
	scriptInstall                  script = "install.sh"
	scriptDisableHostServices     script = "disable-host-services.sh"
	scriptBootstrap                script = "bootstrap.sh"
	scriptLoadImages               script = "load-images.sh"
	scriptConfigureAuthToken       script = "configure-auth-token.sh" // #nosec G101
	scriptConfigureContainerdProxy script = "configure-containerd-proxy.sh"
	scriptConfigureNodeToken       script = "configure-node-token.sh" // #nosec G101
	scriptJoinCluster              script = "join-cluster.sh"
	scriptWaitAPIServerReady       script = "wait-apiserver-ready.sh"
	scriptDeployManifests          script = "deploy-manifests.sh"
	scriptCreateSentinelBootstrap  script = "create-sentinel-bootstrap.sh"
	scriptConfigureSnapstoreProxy  script = "configure-snapstore-proxy.sh"
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
		scriptInstall:                  mustEmbed(scriptInstall),
		scriptDisableHostServices:     mustEmbed(scriptDisableHostServices),
		scriptBootstrap:                mustEmbed(scriptBootstrap),
		scriptLoadImages:               mustEmbed(scriptLoadImages),
		scriptConfigureAuthToken:       mustEmbed(scriptConfigureAuthToken),
		scriptConfigureContainerdProxy: mustEmbed(scriptConfigureContainerdProxy),
		scriptConfigureNodeToken:       mustEmbed(scriptConfigureNodeToken),
		scriptJoinCluster:              mustEmbed(scriptJoinCluster),
		scriptWaitAPIServerReady:       mustEmbed(scriptWaitAPIServerReady),
		scriptDeployManifests:          mustEmbed(scriptDeployManifests),
		scriptCreateSentinelBootstrap:  mustEmbed(scriptCreateSentinelBootstrap),
		scriptConfigureSnapstoreProxy:  mustEmbed(scriptConfigureSnapstoreProxy),
	}
)
