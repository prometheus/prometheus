package python

import (
	"universe.dagger.io/docker"
)

// Python image default name
_#DefaultName: "python"

// Python image default repository
_#DefaultRepository: "index.docker.io"

// Default python docker image version
_#DefaultVersion: "3.8"

#Image: {
	name:       *_#DefaultName | string
	repository: *_#DefaultRepository | string
	version:    string | *_#DefaultVersion

	// Packages to install 
	packages: [pkgName=string]: version: string | *""

	// Default packages installed on the image
	packages: {
		nodejs: _
		npm:    _
		bash:   _
	}

	_dest: string | *"/src"

	// FIXME Basically a copy of alpine.#Build with different image
	// Should we create a special definition?
	docker.#Build & {
		steps: [
			docker.#Pull & {
				source: "\(repository)/\(name):\(version)-alpine"
			},
			for pkgName, pkg in packages {
				docker.#Run & {
					command: {
						name: "apk"
						args: [
							"add",
							"\(pkgName)\(pkg.version)",
						]
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
