package functionApp

import (
	"universe.dagger.io/docker"
)

#Create: {

	image: docker.#Image

	// Azure FunctionApp version
	version: string

	// Function name
	name: string

	// Function location
	location: string

	// ResourceGroup name
	resourceGroup: name: string

	// Storage name
	storage: name: string

	// Additional arguments
	args: [...string] | *[]

	docker.#Run & {
		input: image
		command: {
			"name": "az"
			flags: {
				functionapp:                   true
				create:                        true
				"--resource-group":            resourceGroup.name
				"--consumption-plan-location": location
				"--name":                      name
				"--storage-account":           storage.name
				"--functions-version":         version
			}
			"args": args
		}
	}
}
