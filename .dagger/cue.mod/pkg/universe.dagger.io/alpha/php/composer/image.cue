package composer

import (
	"universe.dagger.io/docker"
)

// Composer image default name
_#DefaultName: "composer"

// Composer image default repository
_#DefaultRepository: "index.docker.io"

// Co image default version
_#DefaultVersion: "latest"

#Image: {
	name:       *_#DefaultName | string
	repository: *_#DefaultRepository
	version:    *_#DefaultVersion | string

	packages: [pkgName=string]: version: string | *""

	packages: git: _

	docker.#Build & {
		steps: [
			docker.#Pull & {
				source: "\(repository)/\(name):\(version)"
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
