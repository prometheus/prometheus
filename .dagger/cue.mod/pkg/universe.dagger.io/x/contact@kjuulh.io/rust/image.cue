//Deprecated: in favor of universe.dagger.io/alpha package
package rust

import (
	"universe.dagger.io/docker"
)

// rust image default version
_#DefaultVersion: "1.60"

// Pull rust base image
#Image: {
	version: *_#DefaultVersion | string

	packages: [pkgName=string]: version: string | *""

	docker.#Build & {
		steps: [
			docker.#Pull & {
				source: "rust:\(version)-alpine"
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
