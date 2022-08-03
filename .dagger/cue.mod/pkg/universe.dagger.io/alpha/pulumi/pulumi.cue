// Run a Pulumi program
package pulumi

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
	"universe.dagger.io/bash"
)

// Run a `pulumi up`
#Up: {
	// Source code of Pulumi program
	source: dagger.#FS

	// Pulumi version
	version: string | *"latest"

	// Pulumi runtime used for this Pulumi program
	runtime: "dotnet" | "go" | "nodejs" | "python"

	// Name of your Pulumi stack
	// Example: "production"
	stack: string

	// Create the stack if it doesn't exist
	stackCreate: *false | true

	// API token if you want to use Pulumi SaaS state backend
	accessToken?: dagger.#Secret

	// Passphrase if you want to use local state backend (Cached by Dagger in buildkit)
	passphrase?: dagger.#Secret

	// Build a docker image to run the netlify client
	_pull_image: docker.#Pull & {
		source: "pulumi/pulumi-\(runtime):\(version)"
	}

	// Run Pulumi up
	container: bash.#Run & {
		input: *_pull_image.output | docker.#Image
		script: {
			_load: core.#Source & {
				path: "."
				include: ["*.sh"]
			}
			directory: _load.output
			filename:  "up.sh"
		}
		env: {
			PULUMI_STACK:   stack
			PULUMI_RUNTIME: runtime

			if true == stackCreate {
				PULUMI_STACK_CREATE: "1"
			}

			if passphrase != _|_ {
				PULUMI_CONFIG_PASSPHRASE: passphrase
			}
			if accessToken != _|_ {
				PULUMI_ACCESS_TOKEN: accessToken
			}
		}
		workdir: "/src"
		mounts: {
			src: {
				dest:     "/src"
				contents: source
			}
			node_modules: {
				dest:     "/src/node_modules"
				contents: core.#CacheDir & {
					id: "pulumi-npm-cache"
				}
			}
		}
	}
}
