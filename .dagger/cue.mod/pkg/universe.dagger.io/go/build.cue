package go

import (
	"dagger.io/dagger"
	"universe.dagger.io/docker"
)

// Build a go binary
#Build: {
	// Source code
	source: dagger.#FS

	// Target package to build
	package: *"." | string

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
		input:    image
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
			name: "go"
			args: [package]
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
