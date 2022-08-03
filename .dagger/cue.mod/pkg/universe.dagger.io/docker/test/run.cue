package docker

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"

	"universe.dagger.io/docker"
	"universe.dagger.io/alpine"
)

dagger.#Plan & {
	actions: test: run: {
		_build: alpine.#Build & {
			packages: bash: _
		}
		_image: _build.output

		// Test: run a simple shell command
		simpleShell: {
			run: docker.#Run & {
				input: _image
				command: {
					name: "/bin/sh"
					args: ["-c", "echo -n hello world >> /output.txt"]
				}
			}

			verify: core.#ReadFile & {
				input: run.output.rootfs
				path:  "/output.txt"
			}
			verify: contents: "hello world"
		}

		// Test: export a file
		exportFile: {
			run: docker.#Run & {
				input: _image
				command: {
					name: "sh"
					flags: "-c": #"""
						echo -n hello world >> /output.txt
						"""#
				}
				export: files: "/output.txt": string & "hello world"
			}
		}

		// Test: export a directory
		exportDirectory: {
			run: docker.#Run & {
				input: _image
				command: {
					name: "sh"
					flags: "-c": #"""
						mkdir -p /test
						echo -n hello world >> /test/output.txt
						"""#
				}
				export: directories: "/test": _
			}

			verify: core.#ReadFile & {
				input: run.export.directories."/test"
				path:  "/output.txt"
			}
			verify: contents: "hello world"
		}

		// Test: export a secret
		exportSecret: {
			run: docker.#Run & {
				input: _image
				command: {
					name: "sh"
					flags: "-c": """
							echo test1 >> /secret1.txt
							echo test2 >> /secret2.txt
						"""
				}
				export: secrets: {
					"/secret1.txt": _
					"/secret2.txt": _
				}
			}

			verify: docker.#Run & {
				input: _image
				command: {
					name: "sh"
					flags: {
						"-e": true
						"-c": """
							test "$SECRET_ENV" = "test1"
							test "$(cat /secret.txt)" = "test2"
						"""
					}
				}
				env: SECRET_ENV: run.export.secrets."/secret1.txt"
				mounts: secrets: {
					dest:     "/secret.txt"
					contents: run.export.secrets."/secret2.txt"
				}
			}
		}

		// Test: configs overriding image defaults
		configs: {
			_base: docker.#Set & {
				input: _image
				config: {
					user:    "nobody"
					workdir: "/sbin"
					entrypoint: ["sh"]
					cmd: ["-c", "echo -n $0 $PWD $(whoami) > /tmp/output.txt"]
				}
			}

			// check defaults not overriden by image config
			runDefaults: docker.#Run & {
				input: _image
				command: {
					name: "sh"
					flags: "-c": "echo -n $PWD $(whoami) > /output.txt"
				}
				export: files: "/output.txt": "/ root"
			}

			// check image defaults
			imageDefaults: docker.#Run & {
				input: _base.output
				export: files: "/tmp/output.txt": "sh /sbin nobody"
			}

			// check overrides by user
			overrides: docker.#Run & {
				input: _base.output
				entrypoint: ["bash"]
				workdir: "/root"
				user:    "root"
				export: files: "/tmp/output.txt": "bash /root root"
			}
		}
	}
}
