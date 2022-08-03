package terraform

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
)

// Terraform log level
#LogLevel: "off" | "info" | "warn" | "error" | "debug" | "trace"

// Default log level
_#DefaultLogLevel: "off"

// Run `terraform CMD`
#Run: {
	// Terraform source code
	source: dagger.#FS

	// If set to true, the cache will never be triggered
	always: bool | *false

	// Terraform command (i.e. init, plan, apply)
	cmd: string

	// Arguments for the Terraform command (i.e. -var-file, -var)
	cmdArgs: [...string]

	// Arguments set internally from other actions
	withinCmdArgs: [...string]

	// Terraform workspace
	workspace: string | *"default"

	// Data directory (i.e. ./.terraform)
	dataDir: string | *".terraform"

	// Log level
	logLevel: #LogLevel | *_#DefaultLogLevel

	// Log path
	logPath: string | *""

	// Environment variables
	env: [string]: string | dagger.#Secret

	// Environment variables set internally from other actions
	withinEnv: [string]: string | dagger.#Secret

	// Joins user and internal command arguments
	_thisCmdArgs: cmdArgs + withinCmdArgs

	// Joins user and internal environment variables
	_thisEnv: env & withinEnv & {
		TF_WORKSPACE:     workspace
		TF_DATA_DIR:      dataDir
		TF_LOG:           logLevel
		TF_LOG_PATH:      logPath
		TF_IN_AUTOMATION: "true"
		TF_INPUT:         "0"
	}

	// Hashicorp Terraform container
	container: #input: docker.#Image | #Image

	// Run command within a container
	_run: docker.#Build & {
		steps: [
			container.#input,
			docker.#Copy & {
				dest:     "/src"
				contents: source
			},
			docker.#Run & {
				"always": always
				workdir:  "/src"
				command: {
					name: cmd
					args: _thisCmdArgs
				}
				env: _thisEnv
			},
		]
	}

	_afterSource: core.#Subdir & {
		input: _run.output.rootfs
		path:  "/src"
	}

	// Terraform image
	outputImage: _run.output

	// Modified Terraform files
	output: _afterSource.output
}
