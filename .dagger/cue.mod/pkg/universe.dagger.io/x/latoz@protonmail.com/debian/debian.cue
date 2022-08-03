//Deprecated: in favor of universe.dagger.io/alpha package
// Base package for Debian Linux
package debian

import (
	"universe.dagger.io/docker"
)

// Build a Debian Linux container image
#Build: {

	// Debian version to install.
	version: string | *"bookworm-slim@sha256:d035fbd0acb0ad941eef3d95c692dd330947489159b4b87f48bfd6e26d0fefee"

	packages: [pkgName=string]: version: string | *""

	_pkgList: [
		for pkgName, pkg in packages {
			"\(pkgName)\(pkg.version)"
		},
	]

	docker.#Build & {
		steps: [
			docker.#Pull & {
				source: "index.docker.io/debian:\(version)"
			},
			docker.#Run & {
				command: {
					name: "apt-get"
					args: ["update"]
				}
			},
			docker.#Run & {
				command: {
					name: "apt-get"
					args: ["install", ...] + _pkgList
					flags: {
						"-y":                      true
						"--no-install-recommends": true
					}
				}
			},
		]
	}
}
