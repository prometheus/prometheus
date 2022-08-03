//Deprecated: in favor of universe.dagger.io/alpha package
package example

import (
	"dagger.io/dagger"

	"universe.dagger.io/alpine"
	"universe.dagger.io/docker"
	"universe.dagger.io/x/berryphillips@gmail.com/onepassword"
)

// Build an alpine base image
image: alpine.#Build

dagger.#Plan & {
	// Get the values from environment variables
	client: env: {
		OP_ADDRESS:    dagger.#Secret
		OP_EMAIL:      dagger.#Secret
		OP_SECRET_KEY: dagger.#Secret
		OP_PASSWORD:   dagger.#Secret
	}

	// Create a credentials config
	_opCreds: onepassword.#Credentials & {
		address:   client.env.OP_ADDRESS
		email:     client.env.OP_EMAIL
		secretKey: client.env.OP_SECRET_KEY
		password:  client.env.OP_PASSWORD
	}

	actions: {
		// Get the secret from 1Password
		mySecret: onepassword.#Read & {
			credentials: _opCreds
			reference:   onepassword.#Reference & {
				vault: "MyVault"
				item:  "MyItem"
				field: "MyField"
			}
		}

		// Use the secret
		example: docker.#Run & {
			input: image.output

			// You can mount the secret in a file
			mounts: secret: {
				dest:     "/secret.txt"
				contents: mySecret.output
			}

			// or set an environment variable
			env: MY_SECRET: mySecret.output

			command: {
				name: "cat"
				args: [ "/secret.txt"]
			}
		}
	}
}
