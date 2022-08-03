package test

import (
	"dagger.io/dagger"

	"universe.dagger.io/alpine"
	"universe.dagger.io/docker"
	"universe.dagger.io/docker/cli"
)

dagger.#Plan & {
	client: network: "unix:///var/run/docker.sock": connect: dagger.#Socket

	actions: test: {
		run: cli.#Run & {
			host: client.network."unix:///var/run/docker.sock".connect
			command: name: "info"
		}

		differentImage: {
			_cli: docker.#Build & {
				steps: [
					alpine.#Build & {
						packages: "docker-cli": {}
					},
					docker.#Run & {
						command: {
							name: "sh"
							flags: "-c": "echo -n foobar > /test.txt"
						}
					},
				]
			}
			run: cli.#Run & {
				input: _cli.output
				host:  client.network."unix:///var/run/docker.sock".connect
				command: {
					name: "docker"
					args: ["info"]
				}
				export: files: "/test.txt": "foobar"
			}
		}

		// FIXME: test remote connections with `docker:dind` image
		// when we have long running tasks
	}
}
