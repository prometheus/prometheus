//Deprecated: in favor of universe.dagger.io/alpha package
package kustomize

import (
	"universe.dagger.io/bash"
	"universe.dagger.io/docker"
	"dagger.io/dagger"
	"dagger.io/dagger/core"
)

_#kustomizationType: "file" | "directory"

// Kustomize and output kubernetes manifest
#Kustomize: {
	// Kubernetes source
	source: dagger.#FS

	type: _#kustomizationType

	{
		type:          "file"
		kustomization: string

		// FIXME(TomChv): Can be simplify by https://github.com/dagger/dagger/pull/2290
		_writeYaml: core.#WriteFile & {
			input:    dagger.#Scratch
			path:     "/kustomization.yaml"
			contents: kustomization
		}

		run: mounts: "kustomization.yaml": {
			contents: _writeYaml.output
			dest:     "/kustom"
		}
	} | {
		type:          "directory"
		kustomization: dagger.#FS
		run: mounts: "kustomization.yaml": {
			contents: kustomization
			dest:     "/kustom"
		}
	}

	_baseImage: #Image

	run: bash.#Run & {
		input: *_baseImage.output | docker.#Image
		mounts: "/source": {
			dest:     "/source"
			contents: source
		}
		script: contents: #"""
			cp /kustom/kustomization.yaml /source | true
			mkdir -p /output
			kustomize build /source >> /output/result.yaml
			"""#
		export: files: "/output/result.yaml": string
	}

	output: run.export.files."/output/result.yaml"
}
