package golangci

import (
	"universe.dagger.io/docker"
)

// golangci-lint image
#Image: {
	repository: string | *"index.docker.io/golangci/golangci-lint"
	tag:        string | *"latest"

	docker.#Pull & {
		source: "\(repository):\(tag)"
	}
}
