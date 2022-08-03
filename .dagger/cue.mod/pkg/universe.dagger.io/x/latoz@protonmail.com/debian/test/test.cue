//Deprecated: in favor of universe.dagger.io/alpha package
package debian

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
	"universe.dagger.io/x/latoz@protonmail.com/debian"
)

dagger.#Plan & {
	actions: test: {
		// Test: customize debian version
		debianVersion: {
			build: debian.#Build & {
				// install an old version on purpose
				version: "buster@sha256:1b236b48c1ef66fa08535a5153266f4959bf58f948db3e68f7d678b651d8e33a"
			}

			verify: core.#Exec & {
				input: build.output.rootfs
				args: ["grep", "-F", "buster", "/etc/os-release"]
			}
		}

		// Test: install packages
		packageInstall: {
			build: debian.#Build & {
				packages: {
					jq: {}
					curl: {}
				}
			}

			check: docker.#Run & {
				input: build.output
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
