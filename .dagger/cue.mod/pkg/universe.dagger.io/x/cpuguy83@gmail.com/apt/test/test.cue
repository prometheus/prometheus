//Deprecated: in favor of universe.dagger.io/alpha package
package apt

import (
	"dagger.io/dagger"

	"universe.dagger.io/x/cpuguy83@gmail.com/apt"
	"universe.dagger.io/docker"
)

dagger.#Plan & {
	actions: test: {
		// Test: install packages
		packageInstall: {
			_build: docker.#Build & {
				steps: [
					docker.#Pull & {
						source: "docker.io/library/debian:bullseye"
					},
					apt.#Install & {
						packages: {
							jq: {}
							curl: {}
						}
					},
				]
			}

			check: docker.#Run & {
				input: _build.output
				command: {
					name: "sh"
					flags: "-c": """
						jq --version > /jq-version.txt
						curl --version > /curl-version.txt
						"""
				}

				export: files: {
					"/jq-version.txt":   =~"^jq"
					"/curl-version.txt": =~"^curl"
				}
			}
		}
	}
}
