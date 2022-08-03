package test

import (
	"dagger.io/dagger"
	"universe.dagger.io/aws"
	"universe.dagger.io/docker"
)

dagger.#Plan & {
	actions: {
		build: aws.#Build

		getVersion: docker.#Run & {
			always: true
			input:  build.output
			command: {
				name: "sh"
				flags: "-c": "aws --version > /output.txt"
			}
			export: files: "/output.txt": =~"^aws-cli/\(aws.#DefaultCliVersion)"
		}
	}
}
