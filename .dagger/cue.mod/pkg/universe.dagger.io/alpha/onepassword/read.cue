package onepassword

import (
	"dagger.io/dagger/core"

	"universe.dagger.io/docker"
)

// Read a secret by secret reference
#Read: {
	// The 1Password account login credentials
	credentials: #Credentials

	// The reference to the 1Password secret
	reference: #Reference

	_image: docker.#Pull & {
		source: "1password/op:\(_#Version)"
	}

	container: docker.#Run & {
		input: _image.output

		command: {
			name: "bash"
			flags: "-c": "/scripts/read.sh > /tmp/secret.txt"
		}

		export: secrets: "/tmp/secret.txt": _

		_scripts: core.#Source & {
			path: "."
			include: ["*.sh"]
		}

		mounts: scripts: {
			contents: _scripts.output
			dest:     "/scripts"
		}

		env: {
			ADDRESS:    credentials.address
			EMAIL:      credentials.email
			SECRET_KEY: credentials.secretKey
			PASSWORD:   credentials.password
			REFERENCE:  reference.output
		}
	}

	output: container.export.secrets["/tmp/secret.txt"]
}
