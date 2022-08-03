package python

import (
	"universe.dagger.io/docker"
)

#Image: {
	// The python version to use
	version: *"3.10" | string

	// Whether to use the alpine-based image or not
	alpine: *true | false

	docker.#Pull & {
		*{
			alpine: true
			source: "python:\(version)-alpine"
		} | {
			alpine: false
			source: "python:\(version)"
		}
	}
}
