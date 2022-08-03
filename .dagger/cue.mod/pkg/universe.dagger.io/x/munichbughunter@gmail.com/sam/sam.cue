//Deprecated: in favor of universe.dagger.io/alpha package
package sam

import (
	"dagger.io/dagger"
	"universe.dagger.io/docker"
)

_destination: "/var/task"

#DefaultImage: docker.#Pull & {
	source: "amazon/aws-sam-cli-build-image-provided:latest"
}

#Sam: {
	_defaultImage: #DefaultImage
	input:         docker.#Image | *_defaultImage.output

	config: #Config

	docker.#Run & {
		if (config.clientSocket != _|_) {
			mounts: dkr: {
				dest:     "/var/run/docker.sock"
				contents: config.clientSocket
			}
		}
		env: {
			AWS_ACCESS_KEY_ID:     config.accessKey
			AWS_SECRET_ACCESS_KEY: config.secretKey
		}
		workdir: _destination
		command: name: "sam"
	}
}

#Build: {
	config:   #Config
	fileTree: dagger.#FS

	_run: docker.#Build & {
		steps: [
			#DefaultImage,
			docker.#Copy & {
				contents: fileTree
				dest:     _destination
			},
			#Sam & {
				"config": config
				command: args: ["build"]
			},
		]
	}
	output: _run.output
}

#Package: {
	config:   #Config
	fileTree: dagger.#FS

	_package: docker.#Build & {
		steps: [
			#DefaultImage,
			docker.#Copy & {
				contents: fileTree
				dest:     _destination
			},
			#Sam & {
				"config": config
				command: args: ["package", "--template-file", "template.yaml", "--output-template-file", "output.yaml", "--s3-bucket", config.bucket, "--region", config.region]
			},
		]
	}
	output: _package.output
}

#Validate: #Sam & {
	config: _
	command: args: ["validate", "--region", config.region]
}

#DeployZip: #Sam & {
	config: _
	command: args: ["deploy", "--template-file", "output.yaml", "--stack-name", config.stackName, "--capabilities", "CAPABILITY_IAM", "--no-confirm-changeset", "--region", config.region]
}

#Deployment: #Sam & {
	command: args: ["deploy", "--force-upload"]
}
