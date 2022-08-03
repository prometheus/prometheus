package promci

import (
	"encoding/yaml"
	"strconv"
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
)

_#build: {
	_cmd:    string
	_source: dagger.#FS
	_binaries: [string]

	_promu: core.#ReadFile & {
		"input": _source
		"path":  ".promu.yml"
	}
	_goVersion: strconv.FormatFloat(yaml.Unmarshal(_promu.contents).go.version, 102, 2, 64)

	_image: docker.#Pull & {
		"source": "quay.io/prometheus/golang-builder:" + _goVersion + "-base"
	}

	_app: docker.#Copy & {
		"input":    _image.output
		"contents": _source
		"dest":     "/app"
	}

	docker.#Run & {
		"input": _app.output
		entrypoint: ["/bin/bash", "-c"]
		command: name: _cmd
		env: {
			GOMODCACHE:       _modCachePath
			npm_config_cache: _npmCachePath
		}
		_modCachePath:   "/go-mod-cache"
		_buildCachePath: "/go-build-cache"
		_npmCachePath:   "/npm-build-cache"
		mounts: {
			"go mod cache": {
				contents: core.#CacheDir & {
					id: "go_mod"
				}
				dest: _modCachePath
			}
			"npm cache": {
				contents: core.#CacheDir & {
					id: "npm"
				}
				dest: _npmCachePath
			}
			"go build cache": {
				contents: core.#CacheDir & {
					id: "go_build"
				}
				dest: _buildCachePath
			}
		}
		workdir: "/app"
	}
}

#Build: {
	source:   dagger.#FS
	cmd:      string
	binaries: [string] | *[]

	_build: _#build & {
		_source:   source
		_cmd:      cmd
		_binaries: binaries
	}
	if len(binaries) == 0 {
		output: _build.output
	}

	if len(binaries) > 0 {
		_export: core.#Copy & {
			input:    dagger.#Scratch
			contents: _build.output.rootfs
			source:   "/app/prometheus"
			dest:     "/prometheus"
		}

		output: _export.output
	}
}
