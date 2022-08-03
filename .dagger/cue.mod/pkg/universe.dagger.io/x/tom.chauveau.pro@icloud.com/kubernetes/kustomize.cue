//Deprecated: in favor of universe.dagger.io/alpha package
package kubernetes

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"

	"universe.dagger.io/docker"
)

_#kustomizationType: "inline" | "directory" | "url" | "manual"

// Build manifest from kustomization
#Kustomize: {
	// Kubernetes manifests source
	source: dagger.#FS

	// Type of kustomization
	// - inline: inline kustomization file
	// - directory: source already contains kustomization.yaml
	// - url: build from GitHub
	// - manual: configure execution manually
	type: _#kustomizationType

	{
		type: "inline"

		// Raw Kustomization
		kustomization: string

		_source: core.#WriteFile & {
			input:    source
			path:     "/kustomization.yaml"
			contents: kustomization
		}

		run: mounts: kustomization: {
			contents: _source.output
			dest:     "/source"
		}
	} | {
		type: "directory"

		// Directory that contains kustomization
		// It will be merged with source
		kustomization: dagger.#FS

		_source: core.#Merge & {
			inputs: [source, kustomization]
		}

		run: mounts: kustomization: {
			type:     "fs"
			contents: _source.output
			dest:     "/source"
		}
	} | {
		type: "url"

		source: dagger.#Scratch

		// Url of the kustomization
		// E.g., https://github.com/kubernetes-sigs/kustomize.git/examples/helloWorld?ref=v1.0.6
		url: string

		run: command: args: [url]
	} | *{
		type: "manual"
	}

	_baseImage: #Kubectl

	run: docker.#Run & {
		user:    "root"
		input:   *_baseImage.output | docker.#Image
		workdir: "/source"
		command: {
			name: "kustomize"
			flags: "-o": "/result.txt"
		}
		export: files: "/result.txt": string
	}

	result: run.export.files."/result.txt"
}
