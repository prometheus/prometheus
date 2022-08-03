// Maintainers: tom.chauveau.pro@icloud.com
package scaleway

import (
	"universe.dagger.io/docker"
)

_#defaultVersion: "v2.4.0"

// Scaleway CLI image
#Image: {
	version: *_#defaultVersion | string

	docker.#Pull & {
		source: "index.docker.io/scaleway/cli:\(version)"
	}
}
