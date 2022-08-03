package go

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"

	"universe.dagger.io/go"
	"universe.dagger.io/docker"
	"universe.dagger.io/alpine"
)

dagger.#Plan & {
	client: filesystem: "./data/hello": read: contents: dagger.#FS

	actions: test: {
		_baseImage: alpine.#Build

		simple: {
			build: go.#Build & {
				source: client.filesystem."./data/hello".read.contents
			}

			exec: docker.#Run & {
				input: _baseImage.output
				command: {
					name: "/bin/sh"
					args: ["-c", "/bin/hello >> /output.txt"]
				}
				env: NAME: "dagger"
				mounts: binary: {
					dest:     "/bin/hello"
					contents: build.output
					source:   "/testgreet"
				}
				export: files: "/output.txt": string & "Hi dagger!"
			}
		}

		withPackage: {
			build: go.#Build & {
				source:  client.filesystem."./data/hello".read.contents
				package: "."
			}
			exec: docker.#Run & {
				input: _baseImage.output
				command: {
					name: "/bin/sh"
					args: ["-c", "/bin/hello >> /output.txt"]
				}
				env: NAME: "dagger"
				mounts: binary: {
					dest:     "/bin/hello"
					contents: build.output
					source:   "/testgreet"
				}
				export: files: "/output.txt": string & "Hi dagger!"
			}
		}

		withPackages: {
			build: go.#Build & {
				source: client.filesystem."./data/hello".read.contents
				packages: ["."]
			}
			exec: docker.#Run & {
				input: _baseImage.output
				command: {
					name: "/bin/sh"
					args: ["-c", "/bin/hello >> /output.txt"]
				}
				env: NAME: "dagger"
				mounts: binary: {
					dest:     "/bin/hello"
					contents: build.output
					source:   "/testgreet"
				}
				export: files: "/output.txt": string & "Hi dagger!"
			}
		}

		customImage: {
			build: go.#Build & {
				source: client.filesystem."./data/hello".read.contents

				_image: go.#Image & {
					version: "1.18"
				}
				image: _image.output
			}

			exec: docker.#Run & {
				input: _baseImage.output
				command: {
					name: "/bin/sh"
					args: ["-c", "/bin/hello >> /output.txt"]
				}
				env: NAME: "dagger"
				mounts: binary: {
					dest:     "/bin/hello"
					contents: build.output
					source:   "/testgreet"
				}
			}

			verify: core.#ReadFile & {
				input: exec.output.rootfs
				path:  "/output.txt"
			} & {
				contents: "Hi dagger!"
			}
		}
		customPath: {
			build: go.#Build & {
				source: client.filesystem."./data/hello".read.contents

				_image: go.#Image & {
					version: "1.18"
				}
				image:      _image.output
				binaryName: "greeter"
			}
			exec: docker.#Run & {
				input: _baseImage.output
				command: {
					name: "/bin/sh"
					args: ["-c", "/bin/greeter >> /output.txt"]
				}
				env: NAME: "dagger"
				mounts: binary: {
					dest:     "/bin/greeter"
					contents: build.output
					source:   "/greeter"
				}
			}
			verify: core.#ReadFile & {
				input: exec.output.rootfs
				path:  "/output.txt"
			} & {
				contents: "Hi dagger!"
			}
		}
	}
}
