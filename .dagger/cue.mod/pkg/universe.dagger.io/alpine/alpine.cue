// Base package for Alpine Linux
package alpine

import (
	"universe.dagger.io/docker"
)

// Build an Alpine Linux container image
#Build: {

	// Alpine version to install.
	version: string | *"3.15.0@sha256:21a3deaa0d32a8057914f36584b5288d2e5ecc984380bc0118285c70fa8c9300"

	// List of packages to install
	packages: [pkgName=string]: {
		// NOTE(samalba, gh issue #1532):
		//   it's not recommended to pin the version as it is already pinned by the major Alpine version
		//   version pinning is for future use (as soon as we support custom repositories like `community`,
		//   `testing` or `edge`)
		version: string | *""
	}

	docker.#Build & {
		steps: [
			docker.#Pull & {
				source: "index.docker.io/alpine:\(version)"
			},
			for pkgName, pkg in packages {
				docker.#Run & {
					command: {
						name: "apk"
						args: ["add", "\(pkgName)\(pkg.version)"]
						flags: {
							"-U":         true
							"--no-cache": true
						}
					}
				}
			},
		]
	}
}
