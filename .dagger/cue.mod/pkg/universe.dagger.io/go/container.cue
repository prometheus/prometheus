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

	// Use go image
	_image: #Image

	_sourcePath:     "/src"
	_modCachePath:   "/root/.cache/go-mod"
	_buildCachePath: "/root/.cache/go-build"

	docker.#Run & {
		input:   *_image.output | docker.#Image
		workdir: _sourcePath
		mounts: {
			"source": {
				dest:     _sourcePath
				contents: source
			}
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
