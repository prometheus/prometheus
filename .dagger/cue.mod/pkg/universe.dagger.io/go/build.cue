package go

import (
	"dagger.io/dagger"
	"universe.dagger.io/docker"
)

// Build a go binary
#Build: {
	// Source code
	source: dagger.#FS

	// DEPRECATED: use packages instead
	package: string | *null

	// Target package to build
	packages: [...string]

	// Target architecture
	arch?: string

	// Target OS
	os?: string

	// Build tags to use for building
	tags: *"" | string

	// LDFLAGS to use for linking
	ldflags: *"" | string

	env: [string]: string

	// Custom go image
	image: *#Image.output | docker.#Image

	// Custom binary name
	binaryName: *"" | string

	container: #Container & {
		"source": source
		"image":  image
		"env": {
			env
			if os != _|_ {
				GOOS: os
			}
			if arch != _|_ {
				GOARCH: arch
			}
		}
		command: {
			//FIXME: find a better workaround with disjunction
			//FIXME: factor with the part from test.cue
			_packages: [...string]
			if package == null && len(packages) == 0 {
				_packages: ["."]
			}
			if package != null && len(packages) == 0 {
				_packages: [package]
			}
			if package == null && len(packages) > 0 {
				_packages: packages
			}
			if package != null && len(packages) > 0 {
				_packages: [package] + packages
			}

			name: "go"
			args: _packages
			flags: {
				build:      true
				"-v":       true
				"-tags":    tags
				"-ldflags": ldflags
				"-o":       "/output/\(binaryName)"
			}
		}
		export: directories: "/output": _
	}

	// Directory containing the output of the build
	output: container.export.directories."/output"
}
