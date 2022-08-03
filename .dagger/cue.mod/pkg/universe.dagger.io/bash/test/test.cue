package bash

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"

	"universe.dagger.io/docker"
	"universe.dagger.io/bash"
)

dagger.#Plan & {
	actions: test: {

		_pull: docker.#Pull & {
			source: "index.docker.io/debian"
		}
		_image: _pull.output

		runSimple: {
			run: bash.#RunSimple & {
				script: contents: """
					echo "hello, there!" > /out.txt
					"""
				export: files: "/out.txt": string
			}
			output: run.export.files."/out.txt" & "hello, there!\n"
		}

		// Run a script from source directory + filename
		runFile: {

			dir:   _load.output
			_load: core.#Source & {
				path: "./data"
				include: ["*.sh"]
			}

			run: bash.#Run & {
				input: _image
				export: files: "/out.txt": _
				script: {
					directory: dir
					filename:  "hello.sh"
				}
			}
			output: run.export.files."/out.txt" & "Hello, world\n"
		}

		// Run a script from string
		runString: {
			run: bash.#Run & {
				input: _image
				export: files: "/output.txt": _
				script: contents: "echo 'Hello, inlined world!' > /output.txt"
			}
			output: run.export.files."/output.txt" & "Hello, inlined world!\n"
		}

	}
}
