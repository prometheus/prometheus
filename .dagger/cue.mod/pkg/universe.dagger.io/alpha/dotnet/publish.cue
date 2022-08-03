package dotnet

import (
	"dagger.io/dagger"
)

// Publish a dotnet binary
#Publish: {
	// Source code
	source: dagger.#FS

	// Target package to publish
	package: *"." | string

	env: [string]: string

	container: #Container & {
		"source": source
		"env": {
			env
		}
		command: {
			args: [package]
			flags: {
				publish: true
				"-o":    "/output/"
			}
		}
		export: directories: "/output": _
	}

	// Directory containing the output of the publish
	output: container.export.directories."/output"
}
