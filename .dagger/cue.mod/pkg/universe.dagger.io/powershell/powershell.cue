// Helpers to run PowerShell commands in containers
package powershell

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
)

// Run a PowerShell (pwsh) script in a Docker container
//  This does not suppore Windows containers or Windows PowerShell.
//  Since this is a thin wrapper over docker.#Run, we embed it.
//  Whether to embed or wrap is a case-by-case decision, like in Go.
#Run: {
	// The script to execute
	script: {
		// A directory containing one or more PowerShell scripts
		directory: dagger.#FS

		// Name of the file to execute
		filename: string

		_directory: directory
		_filename:  filename
	} | {
		// Script contents
		contents: string

		_filename: "run.ps1"
		_write:    core.#WriteFile & {
			input:      dagger.#Scratch
			path:       _filename
			"contents": contents
		}
		_directory: _write.output
	}

	// Arguments to the script
	args: [...string]

	// Where in the container to mount the scripts directory
	_mountpoint: "/powershell/scripts"

	docker.#Run & {
		command: {
			name:   "pwsh"
			"args": args
			flags: "-File": "\(_mountpoint)/\(script._filename)"
		}
		mounts: "Pwsh scripts": {
			contents: script._directory
			dest:     _mountpoint
		}
	}
}
