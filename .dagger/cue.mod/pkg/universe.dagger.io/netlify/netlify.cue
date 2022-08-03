// Deploy to Netlify
// https://netlify.com
package netlify

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"

	"universe.dagger.io/alpine"
	"universe.dagger.io/docker"
	"universe.dagger.io/bash"
)

// Deploy a site to Netlify
#Deploy: {
	// Contents of the site
	contents: dagger.#FS

	// Name of the Netlify site
	// Example: "my-super-site"
	site: string

	// Netlify API token
	token: dagger.#Secret

	// Name of the Netlify team (optional)
	// Example: "acme-inc"
	// Default: use the Netlify account's default team
	team: string | *""

	// Domain at which the site should be available (optional)
	// If not set, Netlify will allocate one under netlify.app.
	// Example: "www.mysupersite.tld"
	domain: string | *null

	// Create the site if it doesn't exist
	create: *true | false

	// Build a docker image to run the netlify client
	_build: docker.#Build & {
		steps: [
			alpine.#Build & {
				packages: {
					bash: {}
					curl: {}
					jq: {}
					npm: {}
				}
			},
			// FIXME: make this an alpine custom package, that would be so cool.
			docker.#Run & {
				command: {
					name: "npm"
					args: ["-g", "install", "netlify-cli@8.6.21"]
				}
			},
		]
	}

	// Run the netlify client in a container
	container: bash.#Run & {
		input: *_build.output | docker.#Image
		script: {
			_load: core.#Source & {
				path: "."
				include: ["*.sh"]
			}
			directory: _load.output
			filename:  "deploy.sh"
		}

		always: true
		env: {
			NETLIFY_SITE_NAME: site
			if (create) {
				NETLIFY_SITE_CREATE: "1"
			}
			if domain != null {
				NETLIFY_DOMAIN: domain
			}
			NETLIFY_ACCOUNT: team
		}
		workdir: "/src"
		mounts: {
			"Site contents": {
				dest:       "/src"
				"contents": contents
			}
			"Netlify token": {
				dest:     "/run/secrets/token"
				contents: token
			}
		}

		export: files: {
			"/netlify/url":       _
			"/netlify/deployUrl": _
			"/netlify/logsUrl":   _
		}
	}

	// URL of the deployed site
	url: container.export.files."/netlify/url"

	// URL of the latest deployment
	deployUrl: container.export.files."/netlify/deployUrl"

	// URL for logs of the latest deployment
	logsUrl: container.export.files."/netlify/logsUrl"
}
