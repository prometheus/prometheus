package account

import (
	"universe.dagger.io/docker"
)

// Create a storage account
#Create: {

	image: docker.#Image

	// ResourceGroup name
	resourceGroup: name: string

	// StorageAccount location
	location: string

	// StorageAccount name
	name: string

	// Additional arguments
	args: [...string] | *[]

	_run: docker.#Run & {
		input: image
		command: {
			"name": "az"
			flags: {
				storage: true
				account: true
				create:  true
				"-n":    name
				"-g":    resourceGroup.name
				"-l":    location
			}
			"args": args
		}
	}

	output: _run.output
}
