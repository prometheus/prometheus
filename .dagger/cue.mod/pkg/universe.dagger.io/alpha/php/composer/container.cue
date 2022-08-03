// Composer is a package manager for PHP applications
package composer

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"

	"universe.dagger.io/docker"
)

// Run a composer install command
#Container: {
	// Container name
	name: *"php_composer" | string

	// Source code
	source: dagger.#FS

	// Use composer image
	_image: #Image

	_sourcePath:        "/output"
	_composerCachePath: "/cache/composer"

	_copy: docker.#Copy & {
		input:    _image.output | docker.#Image
		dest:     _sourcePath
		contents: source
		include: ["composer.json", "composer.lock", "vendor*"]
	}

	docker.#Run & {
		input:   _copy.output
		workdir: _sourcePath
		mounts: "composer cache": {
			contents: core.#CacheDir & {
				id: "\(name)_cache"
			}
			dest: _composerCachePath
		}
		env: COMPOSER_CACHE_DIR: _composerCachePath
	}
}
