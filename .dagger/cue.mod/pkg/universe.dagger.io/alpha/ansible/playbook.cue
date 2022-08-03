package ansible

import (
	"dagger.io/dagger"
	"universe.dagger.io/bash"
)

#SSHKeys: {
	// Folder containing ssh keys to mount [required]
	source: dagger.#FS

	// Folder path where to mount keys in the running container (ie: the one referenced in the Playbook configuration) [default="/root/.ssh"]
	destPath: string | *"/root/.ssh"
}

#Inventory: {
	// Folder where the inventory file is located [required]
	source: dagger.#FS

	// Name of the inventory file [required]
	file: string
}

#PlaybookSource: {
	// Folder containing the ansible project sources (with tasks, vars, etc.) [required]
	source: dagger.#FS

	// Main playbook relative path to run in the project directory [default="main.yml"]
	mainPlaybook: string | *"main.yml"

	// Output path of the playbook generated files [default = "./output"]
	outputPath: string | *"/output"
}

#Playbook: {
	// Ansible Config for running the playbook [required]
	config: #Config

	// Playbook to run [required]
	playbookSource: #PlaybookSource

	// Inventory to use for the playbook [required]
	inventory: #Inventory | *null

	// List of the type ["key1=value1"] [default=null]
	extraVars: [...string] | *null

	// Log level: 1 -> -v, 2 -> -vv, 3 -> vvv [default=0]
	logLevel: int | *0

	// SSH keys of the hosts 
	sshKeys?: #SSHKeys

	_inventoryArg: #InventoryArg & {
		inventoryInput: inventory
	}
	_extraVarArg: #ExtraVarsArg & {
		extraVarsInput: extraVars
	}
	_logLevelArg: #LogLevelArg & {
		logLevelInput: logLevel
	}

	_run: bash.#Run & {
		input: config.output
		mounts: {
			source_mnt: {
				dest:     "/home/source"
				contents: playbookSource.source
			}
			if sshKeys != _|_ {
				ssh_mnt: {
					dest:     sshKeys.destPath
					contents: sshKeys.source
				}
			}
			if inventory != null {
				inventory_mnt: {
					dest:     "/home/inventory"
					contents: inventory.source
				}
			}
		}
		workdir: "/home"
		env: {
			INVENTORY_ARG: _inventoryArg.arg
			EXTRAVAR_ARG:  _extraVarArg.arg
			LOGLEVEL_ARG:  _logLevelArg.arg
			MAIN_PLAYBOOK: playbookSource.mainPlaybook
			OUTPUT_PATH:   playbookSource.outputPath
		}
		script: contents: """
		mkdir -p \(playbookSource.outputPath)
		ansible-playbook $INVENTORY_ARG $EXTRAVAR_ARG $LOGLEVEL_ARG source/$MAIN_PLAYBOOK > $OUTPUT_PATH/ansible-dagger-log.txt
		"""
		export: directories: "\(playbookSource.outputPath)": dagger.#FS
	}
	output: _run.export.directories["\(playbookSource.outputPath)"]
}
