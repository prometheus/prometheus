package python

import (
	"strconv"
	"strings"
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
	"universe.dagger.io/python"
)

dagger.#Plan & {
	actions: test: {

		"python.cue": {
			// Run with a custom path to python
			customPath: {
				run: python.#Run & {
					command: name:    "python3"
					script: contents: "print('Hello, world!')"
				}
				// This needs no output test because it is only testing that the command runs
			}

			// Run a script from source directory + filename
			runFile: {

				dir:   _load.output
				_load: core.#Source & {
					path: "./data"
					include: ["*.py"]
				}

				run: python.#Run & {
					export: files: "/out.txt": _
					script: {
						directory: dir
						filename:  "helloworld.py"
					}
				}
				output: run.export.files."/out.txt" & "Hello, world\n"
			}

			// Run a script from string
			runString: {
				run: python.#Run & {
					export: files: "/output.txt": _
					script: contents: """
						with open("output.txt", 'w') as f:
							f.write("Hello, inlined world!\\n")
						"""
				}
				output: run.export.files."/output.txt" & "Hello, inlined world!\n"
			}
		}

		"image.cue": version: {
			[pythonVersion=string]: [useAlpine=string]: {
				image: python.#Image & {
					version: pythonVersion
					alpine:  strconv.ParseBool(useAlpine)
				}
				check: {
					output: {
						os_release: {
							if image.alpine {
								strings.TrimSpace(_run.export.files."os-release.txt") & "alpine"
							}
							if !image.alpine {
								strings.TrimSpace(_run.export.files."os-release.txt") & "debian"
							}
						}
						python_version: {
							strings.TrimSpace(_run.export.files."python-version.txt") & pythonVersion
						}
					}
					_run: docker.#Run & {
						input: image.output
						command: {
							name: "sh"
							flags: "-c": """
						cat /etc/os-release | grep ^ID | cut -d= -f2 >os-release.txt
						python -V | cut -d' ' -f2 | cut -d. -f1-2 >python-version.txt
						"""
						}
						export: files: "os-release.txt":     _
						export: files: "python-version.txt": _
					}
				}
			}
			"3.10": true: _
			"3.9": false: _
		}
	}
}
