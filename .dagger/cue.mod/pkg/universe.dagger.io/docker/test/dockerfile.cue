package docker

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
)

dagger.#Plan & {
	client: filesystem: "./testdata": read: contents: dagger.#FS

	actions: test: dockerfile: {
		simple: {
			build: docker.#Build & {
				steps: [
					docker.#Dockerfile & {
						source: dagger.#Scratch
						dockerfile: contents: """
							FROM alpine:3.15

							RUN echo -n hello world >> /test.txt
						"""
					},
					docker.#Run & {
						command: {
							name: "/bin/sh"
							args: ["-c", """
						  # Verify that docker.#Dockerfile correctly connect output
						  # into other steps
							grep -q "hello world" /test.txt
						"""]
						}
					},
				]
			}

			verify: core.#ReadFile & {
				input: build.output.rootfs
				path:  "/test.txt"
			} & {
				contents: "hello world"
			}
		}

		withInput: {
			build: docker.#Build & {
				steps: [
					docker.#Dockerfile & {
						source: client.filesystem."./testdata".read.contents
					},
					docker.#Run & {
						command: {
							name: "/bin/sh"
							args: ["-c", """
							hello >> /test.txt
						"""]
						}
					},
				]
			}

			verify: core.#ReadFile & {
				input: build.output.rootfs
				path:  "/test.txt"
			} & {
				contents: "hello world"
			}
		}
	}
}
