package goreleaser

import (
	"universe.dagger.io/docker"
)

// GoReleaser image
#Image: {
	repository: string | *"index.docker.io/goreleaser/goreleaser"
	tag:        string | *"latest"

	docker.#Pull & {
		source: "\(repository):\(tag)"
	}
}
