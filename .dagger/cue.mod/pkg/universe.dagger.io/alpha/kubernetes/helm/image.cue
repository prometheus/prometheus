package helm

import (
	"universe.dagger.io/docker"
)

#Image: docker.#Pull & {
	version: string | *"3.9.1"
	// https://hub.docker.com/r/alpine/helm/tags
	source: "index.docker.io/alpine/helm:\(version)"
}
