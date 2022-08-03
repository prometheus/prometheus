//Deprecated: in favor of universe.dagger.io/alpha package
package kubectl

import (
	"dagger.io/dagger"
	"universe.dagger.io/docker"
)

#Apply: {
	manifests:        dagger.#FS
	kubeconfigSecret: dagger.#Secret

	// Kubernetes version
	version: string | *"latest"

	_pull_image: docker.#Pull & {
		source: "bitnami/kubectl:\(version)"
	}

	apply: docker.#Run & {
		input:   *_pull_image.output | docker.#Image
		workdir: "/work"
		entrypoint: ["kubectl"]
		command: {
			name: "apply"
			args: [
				"--kubeconfig",
				"/kubeconfig",
				"-f",
				"/work",
			]
		}
		mounts: {
			kubeconfig: {
				dest:     "/kubeconfig"
				contents: kubeconfigSecret
				uid:      1001
			}
			work: {
				dest:     "/work"
				contents: manifests
			}
		}
	}
}
