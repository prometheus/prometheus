//Deprecated: in favor of universe.dagger.io/alpha package
package rawkode_kubernetes_example

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/x/david@rawkode.dev/kubernetes:kubectl"
)

dagger.#Plan & {
	client: {
		filesystem: "./": read: contents: dagger.#FS
		commands: kubeconfig: {
			name: "kubectl"
			args: ["config", "view", "--raw"]
			stdout: dagger.#Secret
		}
	}

	actions: rawkode: kubectl.#Apply & {
		_contents: core.#Subdir & {
			input: client.filesystem."./".read.contents
			path:  "/kubernetes"
		}
		manifests:        _contents.output
		kubeconfigSecret: client.commands.kubeconfig.stdout
	}
}
