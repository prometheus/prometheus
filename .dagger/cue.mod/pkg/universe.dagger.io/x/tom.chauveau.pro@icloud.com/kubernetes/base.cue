//Deprecated: in favor of universe.dagger.io/alpha package
package kubernetes

import (
	"dagger.io/dagger"
	"universe.dagger.io/docker"
)

// Location of source to apply
_#location: "directory" | "url" | "kustomization" | "manual"

// Action to execute
_#action: "apply" | "delete"

// Base command for `#Apply` and `#Delete`
_#base: {
	// Kubeconfig
	kubeconfig: dagger.#Secret

	// Yaml's sources
	// - files: directory
	// - url: public url
	// - kustomization: directory with kustomization
	// - manual: manually add flags and args to command
	location: _#location

	// Action to execute (apply or delete)
	action: _#action

	// Namespace to target
	namespace: *"default" | string

	{
		location: "directory"

		// Source directory
		source: dagger.#FS

		// Customize docker.#Run
		command: flags: {
			"-R": *true | bool
			"-f": "/manifest"
		}

		mounts: manifest: {
			type:     "fs" // Resolve disjunction
			dest:     "/manifest"
			contents: source
		}
	} | {
		location: "url"

		// Target url
		url: string

		// Customize docker.#Run
		command: flags: {
			"-f": url
			"-R": *true | bool
		}
	} | {
		location: "kustomization"

		// Source directory
		source: dagger.#FS

		// Customize docker.#Run
		command: flags: "-k": "/manifest"

		mounts: manifest: {
			type:     "fs" // Resolve disjunction
			dest:     "/manifest"
			contents: source
		}
	} | {
		location: "manual"
	}

	_baseImage: #Kubectl

	docker.#Run & {
		user:  "root"
		input: *_baseImage.output | docker.#Image
		command: {
			name: action
			flags: "--namespace": namespace
		}
		mounts: "kubeconfig": {
			dest:     "/.kube/config"
			contents: kubeconfig
		}
	}
}
