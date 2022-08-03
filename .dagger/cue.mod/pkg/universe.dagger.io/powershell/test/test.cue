package powershell

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"

	"universe.dagger.io/docker"
	"universe.dagger.io/powershell"
)

dagger.#Plan & {
	actions: test: {

		_pull: docker.#Pull & {
			source: "mcr.microsoft.com/powershell"
		}
		_image: _pull.output

		// Run a script from source directory + filename
		runFile: {

			dir:   _load.output
			_load: core.#Source & {
				path: "./data"
				include: ["*.ps1"]
			}

			run: powershell.#Run & {
				input: _image
				export: files: "/out.txt": _
				script: {
					directory: dir
					filename:  "hello.ps1"
				}
			}
			output: run.export.files."/out.txt" & "Hello world!\n"
		}

		// Run a script from string
		runString: {
			run: powershell.#Run & {
				input: _image
				export: files: "/output.txt": _
				script: contents: "Set-Content -Value 'Hello inline world!' -Path '/output.txt'"
			}
			output: run.export.files."/output.txt" & "Hello inline world!\n"
		}

		// Test args from string
		runStringArg: {
			run: powershell.#Run & {
				input: _image
				export: files: "/output.txt": _
				script: contents: "Set-Content -Value 'Hello arg world!' -Path $($args[0])"
				args: ["/output.txt"]
			}
			output: run.export.files."/output.txt" & "Hello arg world!\n"
		}

		// Test 2 args from string
		runString2Arg: {
			run: powershell.#Run & {
				input: _image
				export: files: "/output.txt": _
				script: contents: "Set-Content -Value \"Hello args $($args[0])\" -Path $($args[1])"
				args: ["world!", "/output.txt"]
			}
			output: run.export.files."/output.txt" & "Hello args world!\n"
		}

	}
}
