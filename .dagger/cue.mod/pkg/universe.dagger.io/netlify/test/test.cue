package netlify

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"

	"universe.dagger.io/docker"
	"universe.dagger.io/netlify"

	"universe.dagger.io/netlify/test/testutils"
)

dagger.#Plan & {
	client: commands: sops: {
		name: "sops"
		args: ["-d", "../../secrets_sops.yaml"]
		stdout: dagger.#Secret
	}

	actions: test: {

		// Configuration common to all tests
		common: {
			testSecrets: core.#DecodeSecret & {
				input:  client.commands.sops.stdout
				format: "yaml"
			}

			token: testSecrets.output.NETLIFY_TOKEN.contents & dagger.#Secret

			marker: "hello world"

			data: core.#WriteFile & {
				input:    dagger.#Scratch
				path:     "index.html"
				contents: marker
			}
		}

		// Test: deploy a simple site to Netlify
		simple: {
			// Deploy to netlify
			deploy: netlify.#Deploy & {
				team:     "dagger-test"
				token:    common.token
				site:     "dagger-test"
				contents: common.data.output
			}

			verify: testutils.#AssertURL & {
				url:      deploy.deployUrl
				contents: common.marker
			}
		}

		// Test: deploy to Netlify with a custom image
		swapImage: {
			// Deploy to netlify
			deploy: netlify.#Deploy & {
				team:     "dagger-test"
				token:    common.token
				site:     "dagger-test"
				contents: common.data.output
				container: input: customImage.output
			}

			customImage: docker.#Build & {
				steps: [
					docker.#Pull & {
						source: "alpine"
					},
					docker.#Run & {
						command: {
							name: "apk"
							args: [
								"add",
								"--no-cache",
								"yarn",
								"bash",
								"rsync",
								"curl",
								"jq",
							]
						}
					},
					docker.#Run & {
						command: {
							name: "yarn"
							args: ["global", "add", "netlify-cli"]
						}
					},
				]
			}

			verify: testutils.#AssertURL & {
				url:      deploy.deployUrl
				contents: common.marker
			}
		}
	}
}
