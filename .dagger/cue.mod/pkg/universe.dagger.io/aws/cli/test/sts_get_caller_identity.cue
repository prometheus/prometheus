package test

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/aws"
	"universe.dagger.io/aws/cli"
)

dagger.#Plan & {
	client: commands: sops: {
		name: "sops"
		args: ["-d", "--extract", "[\"AWS\"]", "../../../secrets_sops.yaml"]
		stdout: dagger.#Secret
	}

	actions: {
		sopsSecrets: core.#DecodeSecret & {
			format: "yaml"
			input:  client.commands.sops.stdout
		}

		getCallerIdentity: cli.#Command & {
			credentials: aws.#Credentials & {
				accessKeyId:     sopsSecrets.output.AWS_ACCESS_KEY_ID.contents
				secretAccessKey: sopsSecrets.output.AWS_SECRET_ACCESS_KEY.contents
			}
			options: region: "us-east-2"
			service: {
				name:    "sts"
				command: "get-caller-identity"
			}
		}

		verify: getCallerIdentity.result & {
			UserId:  !~"^$"
			Account: !~"^$"
			Arn:     !~"^$"
		}
	}
}
