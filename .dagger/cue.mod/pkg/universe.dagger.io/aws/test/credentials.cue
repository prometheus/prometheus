package test

import (
	"encoding/json"
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/aws"
)

dagger.#Plan & {
	client: commands: sops: {
		name: "sops"
		args: ["-d", "--extract", "[\"AWS\"]", "../../secrets_sops.yaml"]
		stdout: dagger.#Secret
	}

	actions: {
		sopsSecrets: core.#DecodeSecret & {
			format: "yaml"
			input:  client.commands.sops.stdout
		}

		getCallerIdentity: aws.#Container & {
			always:      true
			credentials: aws.#Credentials & {
				accessKeyId:     sopsSecrets.output.AWS_ACCESS_KEY_ID.contents
				secretAccessKey: sopsSecrets.output.AWS_SECRET_ACCESS_KEY.contents
			}

			command: {
				name: "sh"
				flags: "-c": "aws --region us-east-2 sts get-caller-identity > /output.txt"
			}

			export: files: "/output.txt": _
		}

		verify: json.Unmarshal(getCallerIdentity.export.files."/output.txt") & {
			UserId:  string & !~"^$"
			Account: =~"^12[0-9]{8}86$"
			Arn:     =~"(12[0-9]{8}86)"
		}
	}
}
