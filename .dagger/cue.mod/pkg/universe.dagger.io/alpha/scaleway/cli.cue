// Maintainers: tom.chauveau.pro@icloud.com
package scaleway

import (
	"dagger.io/dagger"

	"universe.dagger.io/docker"
)

_#configType: "file" | "env"

// Execute a command with scaleway CLI
#CLI: {
	_baseImage: #Image

	insecure: *false | bool

	// Type of configuration
	configType: _#configType

	{
		configType: "file"

		config: dagger.#FS

		env: SCW_CONFIG_PATH: "/config.yaml"

		mounts: "config": {
			type:     "fs" // Resolve disjunction
			dest:     "/config.yaml"
			contents: config
		}
	} | {
		configType: "env"

		env: {
			SCW_ACCESS_KEY:              dagger.#Secret
			SCW_SECRET_KEY:              dagger.#Secret
			SCW_DEFAULT_ORGANIZATION_ID: dagger.#Secret
			SCW_DEFAULT_PROJECT_ID?:     dagger.#Secret
			SCW_DEFAULT_REGION?:         string
			SCW_DEFAULT_ZONE?:           string
			SCW_API_URL?:                string
		}
	}

	docker.#Run & {
		input: *_baseImage.output | docker.#Image
		env: SCW_INSECURE: "\(insecure)"
	}
}
