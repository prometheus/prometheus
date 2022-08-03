package golangci

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"

	"universe.dagger.io/go"
)

// Like #LintBase, but with a pre-configured container image.
#Lint: #LintBase & {
	_image: #Image
	image:  _image.output
}

// Lint using golangci-lint
#LintBase: {
	// Source code
	source: dagger.#FS

	// golangci-lint version
	version: *"1.46" | string

	// Timeout
	timeout: *"5m" | string

	_cachePath: "/root/.cache/golangci-lint"

	go.#Container & {
		name:     "golangci_lint"
		"source": source
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
