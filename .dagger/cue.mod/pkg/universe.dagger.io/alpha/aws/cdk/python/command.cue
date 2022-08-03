package python

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
)

// Run a cdk command (`cdk <ARGS>')
#Command: {
	// Source code for infra
	source: dagger.#FS

	// Arguments to cdk
	args: [...string]

	// Project name, used for cache scoping
	project: string | *"default"

	_requirementsTxt: core.#Subdir & {
		input: source
		path:  "requirements.txt"
	}

	// requirements.txt file
	requirementsTxt: core.#FS | *_requirementsTxt.output

	// aws cdk cli package name
	cdkCliName?: string

	// aws cdk cli version
	cdkCliVersion?: string

	// Install
	_install: #Install & {
		requirementsTxt: requirementsTxt
		"project":       project

		if cdkCliName != _|_ {
			"cdkCliName": cdkCliName
		}

		if cdkCliVersion != _|_ {
			"cdkCliVersion": cdkCliVersion
		}
	}

	// TODO: Use universe.dagger.op/aws.#Credentials once the package supports SSO and container credentials relative uri
	aws: {
		profile?:                         string
		configFolder?:                    dagger.#FS
		containerCredentialsRelativeUri?: string
		accessKey?:                       string
		secretKey?:                       dagger.#Secret
	}

	// Environment variables passed to the container where command is executed
	env: [string]: string | dagger.#Secret

	// Command logs
	logs: container.export.files."/logs"

	// Container where command is executed
	container: docker.#Run & {
		input:   _image.output
		_image:  #Image
		workdir: _sourcePath

		_sourcePath: "/src"

		// Skip cache validation
		always: true

		// Mount source code and language dependencies
		mounts: {
			"source": {
				dest:     _sourcePath
				contents: source
			}
			nodeModules: {
				contents: _install.output.nodeModules
				dest:     "/src/node_modules"
			}
			venv: {
				contents: _install.output.venv
				dest:     "/src/venv"
			}
		}

		// Setup AWS config by conditionally mounting config folder
		if aws.configFolder != _|_ {
			mounts: "AWS config": {
				dest:     "/aws"
				contents: aws.configFolder
				ro:       true
			}
			env: AWS_CONFIG_FILE: "/aws/config"
		}

		// Script args
		"args": args

		// Script to run
		script: {
			_load: core.#Source & {
				path: "."
				include: ["*.sh"]
			}
			directory: _load.output
			filename:  "cdk.sh"
		}

		export: files: {
			// Export command logs
			"/logs": string
		}

		"env": {
			env

			// Conditionally set AWS profile
			if aws.profile != _|_ {
				AWS_PROFILE: aws.profile
			}

			// Conditionally set AWS container credentials uri
			if aws.containerCredentialsRelativeUri != _|_ {
				AWS_CONTAINER_CREDENTIALS_RELATIVE_URI: aws.containerCredentialsRelativeUri
			}

			// Conditionally set AWS Access Key Id
			if aws.accessKey != _|_ {
				AWS_ACCESS_KEY_ID: aws.accessKey
			}

			// Conditionally set AWS Secret Access Key
			if aws.secretKey != _|_ {
				AWS_SECRET_ACCESS_KEY: aws.secretKey
			}
		}
	}

}
