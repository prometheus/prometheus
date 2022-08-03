//Deprecated: in favor of universe.dagger.io/alpha package
package dotnet

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/x/olli.janatuinen@gmail.com/dotnet"
	"universe.dagger.io/docker"
	"universe.dagger.io/alpine"
)

dagger.#Plan & {
	client: filesystem: "./data": read: contents: dagger.#FS

	actions: test: {
		_baseImage: {
			build: alpine.#Build & {
				packages: {
					"ca-certificates": {}
					"krb5-libs": {}
					libgcc: {}
					libintl: {}
					"libssl1.1": {}
					"libstdc++": {}
					zlib: {}
				}
			}
			output: build.output
		}

		simple: {
			publish: dotnet.#Publish & {
				source:  client.filesystem."./data".read.contents
				package: "hello"
			}

			exec: docker.#Run & {
				input: _baseImage.output
				command: {
					name: "/bin/sh"
					args: ["-c", "/app/hello >> /output.txt"]
				}
				env: NAME: "dagger"
				mounts: binary: {
					dest:     "/app"
					contents: publish.output
					source:   "/"
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
