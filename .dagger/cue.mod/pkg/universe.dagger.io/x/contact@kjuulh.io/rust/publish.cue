//Deprecated: in favor of universe.dagger.io/alpha package
package rust

import (
	"dagger.io/dagger"
)

// Publish a rust binary
#Publish: {
	// Source code
	source: dagger.#FS

	env: [string]: string

	container: #Container & {
		"source": source
		"env": {
			env
			CARGO_TARGET_DIR: "/output/"
		}
		command: {
			name: "cargo"
			flags: {
				build:             true
				"--release":       true
				"--manifest-path": "/src/Cargo.toml"
			}
		}
		export: directories: "/output": _
	}

	// Directory containing the output of the publish
	output: container.export.directories."/output"
}
