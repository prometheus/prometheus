package test

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/x/paul@fgtech.fr/ansible"
)

dagger.#Plan & {

	client: filesystem: ".": read: contents:         dagger.#FS
	client: filesystem: "./output": write: contents: actions.playbook.output

	actions: {
		load: {
			sources:     client.filesystem.".".read.contents
			_loadConfig: core.#Source & {
				path: "./config"
				include: ["*.cfg"]
			}
			_loadInventory: core.#Source & {
				path: "./inventory"
				include: ["*"]
			}
			_loadPlaybook: core.#Source & {
				path: "./project"
				include: ["*"]
			}
			_loadSSH: core.#Source & {
				path: "./ssh"
				include: ["*"]
			}
			configSource:    _loadConfig.output
			inventorySource: _loadInventory.output
			playbookSource:  _loadPlaybook.output
			sshSource:       _loadSSH.output
		}
		config: ansible: ansible.#Config & {
			source: load.configSource
		}
		playbook: ansible.#Playbook & {
			config:    config
			inventory: ansible.#Inventory & {
				source: load.inventorySource
				file:   "hosts"
			}
			playbookSource: {
				source:       load.playbookSource
				mainPlaybook: "main.yml"
				outputPath:   "/tmp"
			}
			logLevel: 3
			sshKeys:  ansible.#SSHKeys & {
				source:   load.sshSource
				destPath: "/root/.ssh"
			}
			extraVars: ["person2=Dagger"]
		}
	}
}
