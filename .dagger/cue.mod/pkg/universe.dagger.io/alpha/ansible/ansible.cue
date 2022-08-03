package ansible

import (
	"dagger.io/dagger"
	"universe.dagger.io/alpine"
	"universe.dagger.io/docker"
	"universe.dagger.io/bash"
)

#Config: {
	// Version of Ansible to use [default="2.9"]
	version: string | *"2.9"

	// Ansible config file [optional]
	source?: dagger.#FS

	// Location of the config file [default=/etc/ansible]
	configFileLocation: *"/etc/ansible" | string

	docker.#Build & {
		steps: [
			alpine.#Build & {
				packages: {
					bash: version: "=~5.1"
					gcc: {}
					grep: {}
					"openssl-dev": {}
					"py3-pip": {}
					"python3-dev": {}
					"gpgme-dev": {}
					"libc-dev": {}
					rust: {}
					cargo: {}
				}
			},
			if source != _|_ {
				docker.#Copy & {
					contents: source
					dest:     configFileLocation
				}
			},
			if source == _|_ {
				bash.#Run & {
					env: {
						ANSIBLE_CONFIG_LOCATION: configFileLocation
						DEFAULT_CONFIG:          #DefaultAnsibleConfig
					}
					script: contents: """
						mkdir -p $ANSIBLE_CONFIG_LOCATION
						echo -en $DEFAULT_CONFIG | tee $ANSIBLE_CONFIG_LOCATION/ansible.cfg
						"""
				}
			},
			bash.#Run & {
				env: ANSIBLE_VERSION: version
				script: contents:     "pip3 install ansible==$ANSIBLE_VERSION"
			},
		]
	}
}
