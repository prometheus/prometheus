package doppler_example

import (
	"dagger.io/dagger"
	"universe.dagger.io/alpha/doppler"
)

dagger.#Plan & {
	client: env: DOPPLER_TOKEN: dagger.#Secret

	actions: test: doppler.#FetchSecrets & {
		project:  "cicd-test"
		config:   "test"
		apiToken: client.env.DOPPLER_TOKEN
	}
}
