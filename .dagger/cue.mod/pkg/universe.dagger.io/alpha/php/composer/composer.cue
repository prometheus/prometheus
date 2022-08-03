// Composer is a package manager for PHP applications
package composer

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"

	"universe.dagger.io/alpine"
	"universe.dagger.io/bash"
	"universe.dagger.io/docker"
)

// Run a composer install command
#Run: {
	// Custom name for this command.
	// Assign an app-specific name if there are multiple apps in the same plan.
	name: string | *""

	// App source code
	source: dagger.#FS

	// Optional arguments for the script
	args: [...string]

	_args: args

	_container: #Build: alpine.#Build & {
		packages: {
			bash: {}
			composer: {}
			php8: {}
		}
	}

	_run: docker.#Build & {
		steps: [
			_container.#Build,

			docker.#Copy & {
				dest:     "/src"
				contents: source
			},

			bash.#Run & {
				input:   _container.#Build.output
				workdir: "/src"
				args:    _args

				mounts: "composer cache": {
					dest:     "/cache/composer"
					contents: core.#CacheDir & {
						id: "universe.dagger.io/composer.#Run \(name)"
					}
				}

				script: {
					_load: core.#Source & {
						path: "."
						include: ["*.sh"]
					}
					directory: _load.output
					filename:  "composer.sh"
				}

				env: COMPOSER_CACHE_DIR: "/cache/composer"
			},
		]
	}

	// The final contents of the package after run
	_output: core.#Subdir & {
		input: _run.output.rootfs
		path:  "/src"
	}

	output: _output.output
}
