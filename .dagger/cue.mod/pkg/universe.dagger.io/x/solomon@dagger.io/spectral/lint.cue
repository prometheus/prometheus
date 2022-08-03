//Deprecated: in favor of universe.dagger.io/alpha package
package spectral

import (
	"list"
	"strings"
	"encoding/json"

	"dagger.io/dagger"
	"universe.dagger.io/docker"
)

// A linter message
#Message: {
	code:    string
	message: string
	path: [...string]
	severity: int
	range: {
		start: {
			line:      int
			character: int
		}
		end: {
			line:      int
			character: int
		}
	}
	source: string
}

// Run spectral linter as a Dagger action
#Lint: {
	// Source directory to lint
	source: dagger.#FS | *dagger.#Scratch

	// Machine-readable logs output
	logs: [...#Message] & json.Unmarshal(container.export.files."/output.json")

	// Human-readable text output
	output: strings.Join([
		for _, m in logs {
			m.message
		},
	], "\n")

	// Location of JSON/YAML documents.
	// Can be either a file, a glob or fetchable resource(s) on the web.
	// Example: ["*.json", "*.yaml", "https://github.com/dagger/dagger/docker-compose.yaml"]
	//
	// Defaults to all json and yaml files at the root of the input directory
	documents: [...string] | *["*.json", "*.yaml", "*.yml"]

	// Filename of a spectral ruleset.
	// FIXME: allow passing structured content as well
	// https://meta.stoplight.io/docs/spectral/ZG9jOjI1MTg1-spectral-cli#using-a-ruleset-file
	ruleset?: {
		// Filename of a ruleset file (relative to input dir)
		filename: string
	}

	// Pass additional flags to spectral
	flags: [string]: bool | string

	// Some flags are baked in
	flags: {
		if (ruleset.filename) != _|_ {
			"--ruleset": ruleset.filename
		}
		"--output": "/output.json"
		"--format": "json"
		// FIXME: expose actual schema for supported spectral flags?

		// text encoding to use
		"--encoding": *"utf8" | "ascii" | "utf-8" | "utf16le" | "ucs2" | "ucs-2" | "base64" | "latin1"

		// formatters to use for outputting results, more than one can be given joining them with a comma
		"--format": "json" | *"stylish" | "junit" | "html" | "text" | "teamcity" | "pretty"

		// path to a file to pretend that stdin comes from
		"--stdin-filepath"?: string

		// path to custom json-ref-resolver instance
		"--resolver"?: string

		// results of this level or above will trigger a failure exit code [
		"--fail-severity": *"error" | "warn" | "info" | "hint"

		// only output results equal to or greater than --fail-severity
		"--display-only-failures"?: bool

		// do not warn about unmatched formats
		"--ignore-unknown-format"?: bool

		// fail on unmatched glob patterns
		"--fail-on-unmatched-globs"?: bool

		// increase verbosity
		"--verbose"?: bool

		// no logging - output only
		"--quiet"?: bool
	}

	// Setup a Docker container to run the spectral CLI
	container: docker.#Run & {
		// Default container image can be overrided.
		//   'spectral' must be installed and in the PATH.
		input:              docker.#Image | *_buildDefaultImage.output
		_buildDefaultImage: docker.#Pull & {
			source: "stoplight/spectral"
		}
		// Ugly hack to override the entrypoint from the default image
		//  FIXME: how do we tuck this into the scope of the default image?
		entrypoint: []

		workdir: "/src"
		mounts: "source": {
			contents: source
			dest:     "/src"
			ro:       true
		}
		command: {
			name: "sh"
			"flags": "-c": """
				spectral lint "$@" || true
				if [ ! -e /output.json ]; then
					exit 1
				fi
				"""
			args: ["--"] + list.FlattenN([
				for k, v in flags {
					if (v & bool) != _|_ {
						[k]
					}
					if (v & string) != _|_ {
						[k, v]
					}
				},
			], 1) + documents
		}
		export: files: "/output.json": string
	}
}
