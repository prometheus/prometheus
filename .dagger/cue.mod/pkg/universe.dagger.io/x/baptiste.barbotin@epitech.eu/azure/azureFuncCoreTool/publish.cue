package azureFuncCoreTool

import (
	"dagger.io/dagger"
	"universe.dagger.io/docker"
)

#Publish: {

	image: docker.#Image

	// Function name
	name: string

	// Source directory
	source: dagger.#FS

	// Additional arguments
	args: [...string] | *[]

	sleep: string | *"0"

	publish: docker.#Build & {
		steps: [
			docker.#Run & {
				input: image
			},
			docker.#Run & {
				command: {
					name: "sleep"
					flags: "\(sleep)": true
				}
			},
			docker.#Run & {
				workdir: "/src"
				command: {
					"name": "func"
					flags: {
						azure:       true
						functionapp: true
						publish:     name
					}
					"args": args
				}
				mounts: "source": {
					dest:     "/src"
					contents: source
				}
			},
		]
	}
}
