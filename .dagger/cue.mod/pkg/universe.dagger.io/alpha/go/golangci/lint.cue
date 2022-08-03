package golangci

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"

	"universe.dagger.io/docker"
	"universe.dagger.io/go"
)

// Lint using golangci-lint
#Lint: {
	// Source code
	source: dagger.#FS

	// golangci-lint version
	version: *"1.46" | string

	// Timeout
	timeout: *"5m" | string

	_image: docker.#Pull & {
		source: "index.docker.io/golangci/golangci-lint:v\(version)"
	}

	_cachePath: "/root/.cache/golangci-lint"

	go.#Container & {
		name:     "golangci_lint"
		"source": source
		input:    _image.output
		command: {
			name: "golangci-lint"
			flags: {
				run:         true
				"-v":        true
				"--timeout": timeout
			}
		}
		mounts: "golangci cache": {
			contents: core.#CacheDir & {
				id: "\(name)_golangci"
			}
			dest: _cachePath
		}
		env: GOLANGCI_LINT_CACHE: _cachePath
	}
}
