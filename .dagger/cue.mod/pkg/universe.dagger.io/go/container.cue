// Go operation
package go

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
)

// A standalone go environment to run go command
#Container: {
	// Container app name
	name: *"go_builder" | string

	// Source code
	source: dagger.#FS

	// Counter FS not set disjunction error
	_img: #Image

	image: *_img.output | docker.#Image

	_sourcePath:     "/src"
	_modCachePath:   "/root/.cache/go-mod"
	_buildCachePath: "/root/.cache/go-build"

	_copy: docker.#Copy & {
		input:    image
		dest:     _sourcePath
		contents: source
	}

	docker.#Run & {
		input:   _copy.output
		workdir: _sourcePath
		mounts: {
			"go mod cache": {
				contents: core.#CacheDir & {
					id: "\(name)_mod"
				}
				dest: _modCachePath
			}
			"go build cache": {
				contents: core.#CacheDir & {
					id: "\(name)_build"
				}
				dest: _buildCachePath
			}
		}
		env: GOMODCACHE: _modCachePath
	}
}
