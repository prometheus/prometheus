package helm

import (
	"universe.dagger.io/docker"
)

#Image: {
	version: string | *"latest"

	docker.#Pull & {
		source: "index.docker.io/alpine/helm:\(version)"
	}
}
