package docker

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/alpine"
	"universe.dagger.io/bash"
	"universe.dagger.io/docker"
	"universe.dagger.io/docker/cli"
)

dagger.#Plan & {
	client: network: "unix:///var/run/docker.sock": connect: dagger.#Socket

	actions: test: image: {

		// Test: change image config with docker.#Set
		set: {
			image: output: docker.#Image & {
				rootfs: dagger.#Scratch
				config: {
					cmd: ["/bin/sh"]
					env: PATH: "/sbin:/bin"
					onbuild: ["COPY . /app"]
				}
			}
			set: docker.#Set & {
				input: image.output
				config: {
					env: FOO: "bar"
					workdir: "/root"
					onbuild: ["RUN /app/build.sh"]
				}
			}
			verify: set.output.config & {
				env: {
					PATH: "/sbin:/bin"
					FOO:  "bar"
				}
				cmd: ["/bin/sh"]
				workdir: "/root"
				onbuild: [
					"COPY . /app",
					"RUN /app/build.sh",
				]
			}
		}

		// Test: image config behavior is correct
		config: {
			build: core.#Dockerfile & {
				source: dagger.#Scratch
				dockerfile: contents: """
					FROM alpine:3.15.0
					RUN echo -n 'not hello from dagger' > /dagger.txt
					RUN echo '#!/bin/sh' > /bin/dagger
					ENV HELLO_FROM=dagger
					RUN echo 'echo -n "hello from $HELLO_FROM" > /dagger.txt' >> /bin/dagger
					RUN chmod +x /bin/dagger
					WORKDIR /bin
					CMD /bin/dagger
					"""
			}
			myimage: docker.#Image & {
				rootfs: build.output
				config: build.config
			}
			run: docker.#Run & {
				input: myimage
				command: name: "ls"
				export: files: {
					"/dagger.txt": "not hello from dagger"
					"/bin/dagger": """
						#!/bin/sh
						echo -n "hello from $HELLO_FROM" > /dagger.txt

						"""
				}
			}
			verify_cmd_is_run: docker.#Run & {
				input: myimage
				export: files: "/dagger.txt": "hello from dagger"
			}
			verify_env_is_overridden: docker.#Run & {
				input: myimage
				export: files: "/dagger.txt": "hello from europa"
				env: HELLO_FROM: "europa"
			}

			verify_working_directory: docker.#Run & {
				input: myimage
				command: {
					name: "sh"
					flags: "-c": #"""
						pwd > dir.txt
						"""#
				}
				export: files: "/bin/dir.txt": "/bin\n"
			}
			verify_working_directory_is_overridden: docker.#Run & {
				input:   myimage
				workdir: "/"
				command: {
					name: "sh"
					flags: "-c": #"""
						pwd > dir.txt
						"""#
				}
				export: files: "/dir.txt": "/\n"
			}
		}

		// Test: scratch image size is 0 bytes
		scratch: {
			_cli: alpine.#Build & {
				packages: {
					bash: {}
					"docker-cli": {}
				}
			}

			load: cli.#Load & {
				image: docker.#Scratch
				host:  client.network."unix:///var/run/docker.sock".connect
				tag:   "dagger_scratch:load"
			}

			verify_size: bash.#Run & {
				input: _cli.output
				mounts: docker: {
					contents: client.network."unix:///var/run/docker.sock".connect
					dest:     "/var/run/docker.sock"
				}
				env: {
					IMAGE_NAME: load.tag
					IMAGE_ID:   load.imageID
					// FIXME: without this forced dependency, load.command might not run
					DEP: "\(load.success)"
				}
				script: contents: "test $(docker image inspect $IMAGE_NAME -f '{{.Size}}') = 0"
			}
		}
	}
}
