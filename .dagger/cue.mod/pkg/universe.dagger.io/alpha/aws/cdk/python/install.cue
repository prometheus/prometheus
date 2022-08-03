package python

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/bash"
	"universe.dagger.io/docker"
)

// Default aws cdk cli package name
_#DefaultCdkCliName: "aws-cdk"

// Default aws cdk cli version
_#DefaultCdkCliVersion: "2.28.0"

// Install cdk cli (using npm) and cdk library for python (using pip)
#Install: {

	// requirements.txt file
	requirementsTxt: dagger.#FS

	// aws cdk cli package name
	cdkCliName: *_#DefaultCdkCliName | string

	// aws cdk cli version
	cdkCliVersion: *_#DefaultCdkCliVersion | string

	// Project name, used for cache scoping
	project: string | *"default"

	// Output directories
	output: {
		nodeModules: container.export.directories."/node_modules"
		venv:        container.export.directories."/venv"

		// Logs produced by the install script
		logs: container.export.files."/logs"
	}

	image: docker.#Image | *#Image

	// Container where install is executed
	container: bash.#Run & {
		input:   image.output
		workdir: _sourcePath

		env: {
			CDK_CLI_NAME:    cdkCliName
			CDK_CLI_VERSION: cdkCliVersion
		}

		_sourcePath: "/src"

		mounts: "requirementsTxt": {
			dest:     _sourcePath
			contents: requirementsTxt
		}

		// Script to run
		script: {
			_load: core.#Source & {
				path: "."
				include: ["*.sh"]
			}
			directory: _load.output
			filename:  "install.sh"
		}

		export: {
			directories: {
				"/node_modules": dagger.#FS
				"/venv":         dagger.#FS
			}
			files: "/logs": string
		}

		// Setup caching
		env: {
			PIP_CACHE_DIR:      "/cache/pip"
			NPM_CACHE_LOCATION: "/cache/npm"
		}
		mounts: {
			"Pip cache": {
				dest:     "/cache/pip"
				contents: core.#CacheDir & {
					id: "\(project)-pip"
				}
			}
			"Venv cache": {
				dest:     "\(_sourcePath)/venv"
				contents: core.#CacheDir & {
					id: "\(project)-venv"
				}
			}
			"Npm cache": {
				dest:     "/cache/npm"
				contents: core.#CacheDir & {
					id: "\(project)-npm"
				}
			}
			"NodeJS cache": {
				dest:     "\(_sourcePath)/node_modules"
				type:     "cache"
				contents: core.#CacheDir & {
					id: "\(project)-nodejs"
				}
			}
		}
	}
}
