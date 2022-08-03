package apt

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
)

_#aptMounts: {
	cachePrefix:    string
	_confKeepcache: core.#WriteFile & {
		input:    dagger.#Scratch
		contents: "Binary::apt::APT::Keep-Downloaded-Packages \"true\";"
		path:     "/keep-cache"
	}

	_updatedPath: "/var/cache/apt/dagger/updated"

	// Use this to track if `apt-get update` needs to be re-run
	// This works by checking the last update time of the file specified below
	// Since this is being written to a directory where there is a cache mount anyway, it won't persist in the image
	_confUpdateSuccess: core.#WriteFile & {
		input:    dagger.#Scratch
		contents: "APT::Update::Post-Invoke-Success { \"mkdir -p /var/cache/apt/dagger; touch \(_updatedPath)\"; };"
		path:     "/apt-update-invoke-success"
	}

	// Use this to mount over some well-meaning default apt configs that get injected into the base container image.
	// The configs do things like clean up the apt cache after install in order to keep image sizes smaller.
	// For the caching strategy here, it is desirable to keep the apt cache in tact as it is getting written to a cache mount instead of the container fs.
	_emptyFile: core.#WriteFile & {
		input:    dagger.#Scratch
		path:     "/empty"
		contents: ""
	}

	mounts: {
		"no-apt-clean": {
			type:     "fs"
			ro:       true
			contents: _emptyFile.output
			source:   _emptyFile.path
			dest:     "/etc/apt/apt.conf.d/docker-clean"
		}
		"no-apt-gzip": {
			type:     "fs"
			ro:       true
			contents: _emptyFile.output
			source:   _emptyFile.path
			dest:     "/etc/apt/apt.conf.d/docker-gzip-indexes"
		}
		"keep-cache": {
			type:     "fs"
			ro:       true
			contents: _confKeepcache.output
			source:   _confKeepcache.path
			dest:     "/etc/apt/apt.conf.d/dagger-io-keep-cache"
		}
		"update-success": {
			type:     "fs"
			ro:       true
			contents: _confUpdateSuccess.output
			source:   _confUpdateSuccess.path
			dest:     "/etc/apt/apt.conf.d/dagger-io-update-success-trigger"
		}
		"var-cache-apt": {
			contents: core.#CacheDir & {
				id: "\(cachePrefix)-var-cache-apt"
			}
			dest: "/var/cache/apt"
		}
		"var-lib-apt": {
			contents: core.#CacheDir & {
				id: "\(cachePrefix)-var-lib-apt"
			}
			dest: "/var/lib/apt"
		}
	}
}

#Install: {
	input:       docker.#Image
	cachePrefix: string | *""
	packages: [pkgName=string]: {}
	updatedMaxAgeMinutes: int | *60

	_apt: _#aptMounts & {
		"cachePrefix": cachePrefix
	}

	docker.#Build & {
		steps: [
			{output: input},
			for pkgName, pkg in packages {
				docker.#Run & {
					mounts: _apt.mounts
					env: {
						DEBIAN_FRONTEND:        "noninteractive"
						PKG_NAME:               pkgName
						DAGGER_UPDATED_PATH:    _apt._updatedPath
						DAGGER_UPDATED_MAX_AGE: "\(updatedMaxAgeMinutes)"
					}
					command: {
						name: "/bin/sh"
						args: ["-c", """
                            set -e
                            if [ -z "$(find ${DAGGER_UPDATED_PATH} -mmin -${DAGGER_UPDATED_MAX_AGE})" ]; then 
                                apt-get update
                            fi
                            apt-get install -y ${PKG_NAME}
                        """]
					}
				}
			},
		]
	}
}
