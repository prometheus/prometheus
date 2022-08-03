// The doppler package makes it easy to fetch or update secrets using the 
// doppler.com SecretOps platform
package doppler

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
)

// Fetch a Config from Doppler
#FetchConfig: {
	// Doppler can be configured by a `doppler.yaml` file.
	// If you pass a core.#Source, we'll read the `doppler.yaml`
	// file from it.
	configDirectory?: core.#Source

	// Doppler can be configured by a combination of project and config (environment)
	// You can provide these as strings
	project?: string
	config?:  string

	// The token to use in-order to authenticate against the Doppler API
	apiToken: dagger.#Secret

	// Steps
	imageName: string | *"dopplerhq/cli:3"

	_pullImage: docker.#Pull & {
		source: imageName
	}

	_fetchSecrets: docker.#Run & {
		input: _pullImage.output

		env: DOPPLER_TOKEN: apiToken

		if configDirectory != _|_ {
			mounts: "Doppler Config": {
				contents: configDirectory.output
				dest:     "/src"
			}
		}

		if project != _|_ {
			env: DOPPLER_PROJECT: project
		}

		if config != _|_ {
			env: DOPPLER_CONFIG: config
		}

		workdir: "/src"
		entrypoint: ["ash"]
		command: {
			name: "-c"
			args: ["mkdir -p /output && doppler setup --no-save-token --no-interactive && doppler secrets --json > /output/secrets.json"]
		}
	}

	_dopplerOutput: core.#Subdir & {
		input: _fetchSecrets.export.rootfs
		path:  "/output"
	}

	_dopplerOutputSecrets: core.#NewSecret & {
		input: _dopplerOutput.output
		path:  "secrets.json"
	}

	_decodeSecret: core.#DecodeSecret & {
		input:  _dopplerOutputSecrets.output
		format: "json"
	}

	output: _decodeSecret.output
}
