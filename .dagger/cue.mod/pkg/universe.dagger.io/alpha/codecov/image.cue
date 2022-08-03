package codecov

import (
	"universe.dagger.io/bash"
	"universe.dagger.io/docker"
)

// Build a codecov base image
#Image: {
	// Codecov uploader version
	version: string | *"latest"

	// FIXME once nested build works
	// https://github.com/dagger/dagger/issues/1466
	_packages: [pkgName=string]: version: string | *""
	_packages: {
		bash:         _
		curl:         _
		git:          _
		gnupg:        _
		coreutils:    _
		"perl-utils": _
	}

	docker.#Build & {
		steps: [
			// FIXME once nested build works
			// https://github.com/dagger/dagger/issues/1466
			docker.#Pull & {
				source: "index.docker.io/alpine:3.15.0@sha256:21a3deaa0d32a8057914f36584b5288d2e5ecc984380bc0118285c70fa8c9300"
			},
			for pkgName, pkg in _packages {
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
			bash.#Run & {
				script: contents: """
					curl https://keybase.io/codecovsecurity/pgp_keys.asc | gpg --no-default-keyring --keyring trustedkeys.gpg --import
					"""
			},
			bash.#Run & {
				script: contents: """
					curl -Os https://uploader.codecov.io/\(version)/alpine/codecov

					curl -Os https://uploader.codecov.io/\(version)/alpine/codecov.SHA256SUM

					curl -Os https://uploader.codecov.io/\(version)/alpine/codecov.SHA256SUM.sig

					gpgv codecov.SHA256SUM.sig codecov.SHA256SUM

					shasum -a 256 -c codecov.SHA256SUM

					chmod +x codecov

					mv codecov /usr/local/bin
					rm codecov.SHA256SUM codecov.SHA256SUM.sig
					"""
			},
		]
	}
}
