//Deprecated: in favor of universe.dagger.io/alpha package
package kubernetes

import (
	"dagger.io/dagger"

	"universe.dagger.io/x/tom.chauveau.pro@icloud.com/kubernetes"
)

dagger.#Plan & {
	// Kubeconfig must be in current directory
	client: {
		filesystem: {
			"./data/hello-pod": read: contents:       dagger.#FS
			"./data/hello-kustomize": read: contents: dagger.#FS
		}
		commands: kubeconfig: {
			name: "kubectl"
			args: ["config", "view", "--raw"]
			stdout: dagger.#Secret
		}
	}

	actions: test: {
		// Alias on kubeconfig
		_kubeconfig: client.commands.kubeconfig.stdout

		// Alias on hello-pod
		_helloPodSource: client.filesystem."./data/hello-pod".read.contents

		// Alias on hello-kustomize
		_helloKustomSource: client.filesystem."./data/hello-kustomize".read.contents

		apply: {
			url: kubernetes.#Apply & {
				kubeconfig: _kubeconfig
				location:   "url"
				url:        "https://gist.githubusercontent.com/grouville/04402633618f3289a633f652e9e4412c/raw/293fa6197b78ba3fad7200fa74b52c62ec8e6703/hello-world-pod.yaml"
			}

			directory: kubernetes.#Apply & {
				kubeconfig: _kubeconfig
				location:   "directory"
				source:     _helloPodSource
			}

			kustomize: kubernetes.#Apply & {
				kubeconfig: _kubeconfig
				location:   "kustomization"
				source:     _helloKustomSource
			}
		}

		delete: {
			url: kubernetes.#Delete & {
				kubeconfig: _kubeconfig
				location:   "url"
				url:        test.apply.url.url

				// Explicit dependency
				env: APPLY_URL: "\(test.apply.url.success)"
			}

			// Clean up apply.directory
			directory: kubernetes.#Delete & {
				kubeconfig: _kubeconfig
				location:   "directory"
				source:     _helloPodSource

				// Explicit dependency
				env: APPLY_DIR: "\(test.apply.directory.success)"
			}

			kustomize: kubernetes.#Delete & {
				kubeconfig: _kubeconfig
				location:   "kustomization"
				source:     _helloKustomSource

				// Explicit dependency
				env: KUSTOM_DIR: "\(test.apply.kustomize.success)"
			}
		}
	}
}
