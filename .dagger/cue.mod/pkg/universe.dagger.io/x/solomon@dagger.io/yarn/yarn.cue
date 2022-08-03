//Deprecated: in favor of universe.dagger.io/alpha package
// Use [Yarn](https://yarnpkg.com) in a Dagger action
package yarn

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"

	"universe.dagger.io/alpine"
	"universe.dagger.io/bash"
)

// Install dependencies with yarn ('yarn install')
#Install: #Command & {
	args: ["install"]
}

// Build an application with yarn ('yarn run build')
#Build: #Script & {
	name: "build"
}

// Run a yarn script ('yarn run <NAME>')
#Script: {
	// App source code
	source: dagger.#FS

	// Yarn project
	project: string

	// Name of the yarn script to run
	// Example: "build"
	name: string

	#Command & {
		"source":  source
		"project": project
		args: ["run", name]

		// Mount output directory of install command,
		//   even though we don't need it,
		//   to trigger an explicit dependency.
		container: mounts: install_output: {
			contents: install.output
			dest:     "/tmp/yarn_install_output"
		}
	}

	install: #Install & {
		"source":  source
		"project": project
	}

}

// Run a yarn command (`yarn <ARGS>')
#Command: {
	// Source code to build
	source: dagger.#FS

	// Arguments to yarn
	args: [...string]

	// Project name, used for cache scoping
	project: string | *"default"

	// Path of the yarn script's output directory
	// May be absolute, or relative to the workdir
	outputDir: string | *"./build"

	// Output directory
	output: container.export.directories."/output"

	// Logs produced by the yarn script
	logs: container.export.files."/logs"

	container: bash.#Run & {
		"args": args

		input:  *_image.output | _
		_image: alpine.#Build & {
			packages: {
				bash: {}
				yarn: {}
				git: {}
			}
		}

		workdir: "/src"
		mounts: Source: {
			dest:     "/src"
			contents: source
		}
		script: contents: """
			set -x
			yarn "$@" | tee /logs
			echo $$ > /code
			if [ -e "$YARN_OUTPUT_FOLDER" ]; then
				mv "$YARN_OUTPUT_FOLDER" /output
			else
				mkdir /output
			fi
			"""
		export: {
			directories: "/output": dagger.#FS
			files: {
				"/logs": string
				"/code": string
			}
		}

		// Setup caching
		env: {
			YARN_CACHE_FOLDER:  "/cache/yarn"
			YARN_OUTPUT_FOLDER: outputDir
		}
		mounts: {
			"Yarn cache": {
				dest:     "/cache/yarn"
				contents: core.#CacheDir & {
					id: "\(project)-yarn"
				}
			}
			"NodeJS cache": {
				dest:     "/src/node_modules"
				type:     "cache"
				contents: core.#CacheDir & {
					id: "\(project)-nodejs"
				}
			}
		}
	}
}
