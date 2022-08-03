package resourceGroup

import (
	"universe.dagger.io/docker"
)

#Create: {

	image: docker.#Image

	// ResourceGroup name
	name: string

	// ResourceGroup location
	location: string

	// Additional arguments
	args: [...string] | *[]

	_run: docker.#Run & {
		input: image
		command: {
			"name": "az"
			flags: {
				group:  true
				create: true
				"-l":   location
				"-n":   name
			}
			"args": args
		}
	}

	output: _run.output
}
