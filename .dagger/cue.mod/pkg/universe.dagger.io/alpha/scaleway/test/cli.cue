package scaleway

import (
	"dagger.io/dagger"

	"universe.dagger.io/alpha/scaleway"
)

dagger.#Plan & {
	client: env: {
		SCW_ACCESS_KEY:              dagger.#Secret
		SCW_SECRET_KEY:              dagger.#Secret
		SCW_DEFAULT_ORGANIZATION_ID: dagger.#Secret
	}
	actions: test: cli: {
		_env: client.env

		// Execute a simple scaleway command
		simple: scaleway.#CLI & {
			configType: "env"
			env: {
				SCW_ACCESS_KEY:              _env.SCW_ACCESS_KEY
				SCW_SECRET_KEY:              _env.SCW_SECRET_KEY
				SCW_DEFAULT_ORGANIZATION_ID: _env.SCW_DEFAULT_ORGANIZATION_ID
			}
			command: {
				name: "registry"
				args: ["image", "list"]
				flags: "-o": "json"
			}
		}
	}
}
