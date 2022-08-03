//Deprecated: in favor of universe.dagger.io/alpha package
package kubernetes

import (
	"universe.dagger.io/docker"
)

_#DefaultVersion: "1.23.5"

// Kubectl client
#Kubectl: {
	version: *_#DefaultVersion | string

	docker.#Pull & {
		source: "index.docker.io/bitnami/kubectl:\(version)"
	}
}
