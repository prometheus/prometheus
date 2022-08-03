//Deprecated: in favor of universe.dagger.io/alpha package
package kustomize

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/x/wangwei27494731@gmail.com/kustomize"
	"encoding/yaml"
	"universe.dagger.io/bash"
)

dagger.#Plan & {
	client: filesystem: "./testdata": read: contents: dagger.#FS
	actions: test: {
		_baseImage: #Image

		inlineFile: {
			// Run Kustomize
			kustom: kustomize.#Kustomize & {
				source:        client.filesystem."./testdata".read.contents
				kustomization: yaml.Marshal({
					resources: ["deployment.yaml", "pod.yaml"]
					images: [{
						name:   "nginx"
						newTag: "v1"
					}]
					replicas: [{
						name:  "nginx-deployment"
						count: 2
					}]
				})
			}

			_file: core.#WriteFile & {
				input:    dagger.#Scratch
				path:     "/result.yaml"
				contents: kustom.output
			}

			verify: bash.#Run & {
				input: _baseImage.output
				script: contents: #"""
					cat /result/result.yaml
					grep -q "replicas: 2" /result/result.yaml
					"""#
				mounts: "/result": {
					dest:     "/result"
					contents: _file.output
				}
			}
		}

		mountedKustomization: {
			// Test for kustomization FS type
			kustom: kustomize.#Kustomize & {
				source:        client.filesystem."./testdata".read.contents
				kustomization: client.filesystem."./testdata".read.contents
			}

			_file: core.#WriteFile & {
				input:    dagger.#Scratch
				path:     "/result.yaml"
				contents: kustom.output
			}

			verify: bash.#Run & {
				input: _baseImage.output
				script: contents: #"""
					cat /result/result.yaml
					grep -q "replicas: 2" /result/result.yaml
					"""#
				mounts: "/result": {
					dest:     "/result"
					contents: _file.output
				}
			}
		}
	}
}
