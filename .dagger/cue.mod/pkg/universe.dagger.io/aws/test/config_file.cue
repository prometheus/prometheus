package test

import (
	"encoding/json"
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/aws"
)

dagger.#Plan & {
	client: {
		filesystem: ".": read: {
			contents: dagger.#FS
			include: ["config"]
		}
		commands: sops: {
			name: "sops"
			args: ["-d", "--extract", "[\"AWS\"]", "../../secrets_sops.yaml"]
			stdout: dagger.#Secret
		}
	}

	actions: {
		sopsSecrets: core.#DecodeSecret & {
			format: "yaml"
			input:  client.commands.sops.stdout
		}

		getCallerIdentity: aws.#Container & {
			always:     true
			configFile: client.filesystem.".".read.contents

			credentials: aws.#Credentials & {
				accessKeyId:     sopsSecrets.output.AWS_ACCESS_KEY_ID.contents
				secretAccessKey: sopsSecrets.output.AWS_SECRET_ACCESS_KEY.contents
			}

			command: {
				name: "sh"
				flags: "-c": "aws --profile ci sts get-caller-identity > /output.txt"
			}

			export: files: "/output.txt": _
		}

		verify: json.Unmarshal(getCallerIdentity.export.files."/output.txt") & {
			UserId:  string
			Account: =~"^12[0-9]{8}86$"
			Arn:     =~"^arn:aws:sts::(12[0-9]{8}86):assumed-role/dagger-ci"
		}
	}
}
