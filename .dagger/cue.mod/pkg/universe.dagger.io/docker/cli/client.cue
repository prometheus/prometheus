package cli

import (
	"dagger.io/dagger"
	"universe.dagger.io/docker"
)

// See https://github.com/dagger/dagger/discussions/1874

// Default image
#Image: docker.#Pull & {
	source: "docker:20.10.13-alpine3.15"
}

// Run a docker CLI command
#Run: {
	#RunSocket | #RunSSH | #RunTCP

	_defaultImage: #Image

	// As a convenience, input defaults to a ready-to-use docker environment
	input: docker.#Image | *_defaultImage.output
}

// Connect via local docker socket
#RunSocket: {
	host: dagger.#Socket

	docker.#Run & {
		mounts: docker: {
			dest:     "/var/run/docker.sock"
			contents: host
		}
	}
}

// Connect via SSH
#RunSSH: {
	host: =~"^ssh://.+"

	ssh: {
		// Private SSH key
		key?: dagger.#Secret

		// Known hosts file contents
		knownHosts?: dagger.#Secret

		// FIXME: implement keyPassphrase
	}

	docker.#Run & {
		env: DOCKER_HOST: host

		mounts: {
			if ssh.key != _|_ {
				ssh_key: {
					dest:     "/root/.ssh/id_rsa"
					contents: ssh.key
				}
			}
			if ssh.knownHosts != _|_ {
				ssh_hosts: {
					dest:     "/root/.ssh/known_hosts"
					contents: ssh.knownHosts
				}
			}
		}
	}
}

// Connect via HTTP/HTTPS
#RunTCP: {
	host: =~"^tcp://.+"

	// Directory with certificates to verify ({ca,cert,key}.pem files).
	// This enables HTTPS.
	certs?: dagger.#FS

	docker.#Run & {
		env: {
			DOCKER_HOST: host

			if certs != _|_ {
				DOCKER_TLS_VERIFY: "1"
				DOCKER_CERT_PATH:  "/certs/client"
			}
		}
		mounts: {
			if certs != _|_ {
				"certs": {
					dest:     "/certs/client"
					contents: certs
				}
			}
		}
	}
}
