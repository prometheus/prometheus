//Deprecated: in favor of universe.dagger.io/alpha package
package terraform

import (
	"universe.dagger.io/docker"
)

// Terraform image default version
_#DefaultVersion: "1.1.8"

// Terraform base image
#Image: {
	// Terraform version
	version: *_#DefaultVersion | string

	docker.#Build & {
		steps: [
			docker.#Pull & {
				source: "hashicorp/terraform:\(version)"
			},
		]
	}
}
