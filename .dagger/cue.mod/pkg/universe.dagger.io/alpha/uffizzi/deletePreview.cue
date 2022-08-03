// Helpers to create and interact with uffizzi previews
package uffizzi

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"

	"universe.dagger.io/docker"
)

// Update a Delete Preview
#DeletePreview: {

	source: {
		// A directory containing all necessary file for creating theuffizzi preview 
		// (usually the source code and docker compose file of the said project )
		directory: dagger.#FS

		// Name of the files to consider
		filename: string

		_directory: directory
		_filename:  filename
	} | {
		// Directory contents
		contents: string

		_filename: "*"
		_write:    core.#WriteFile & {
			input:      dagger.#Scratch
			path:       _filename
			"contents": contents
		}
		_directory: _write.output
	}

	// Uffizzi username and password (required)
	user:     string
	password: string

	// Uffizzi project name (required)
	project: string

	// Uffizzi server (defaults to https://app.uffizzi.com)
	server: string | *"https://app.uffizzi.com"

	// Uffizzi preview ID (required)
	preview_id: string

	// arguments to uffizzi
	args: [...string]

	// where to mount the script inside the container
	_mountpoint: "/uffizzi_source/"

	docker.#Run & {
		// As a convenience, image defaults to a ready-to-use python environment
		_defaultImage: #Image
		input:         *_defaultImage.output | docker.#Image

		command: {
			name: "preview"
			args: [ "delete", "\(preview_id)"] + args
		}

		mounts: uffizzi_source: {
			contents: source._directory
			dest:     _mountpoint
		}
	}
}
