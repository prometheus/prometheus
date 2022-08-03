// Azure base package
package login

import (
	"dagger.io/dagger"
	"universe.dagger.io/docker"
)

// Default Azure CLI version
#DefaultVersion: "3.0"

#Config: {

	// subscription id corresponds to your Azure active subscription id
	subscriptionId: dagger.#Secret

	// version corresponds to the image version used by the container
	// each version contains a different version of the Azure Func core Tool
	//
	// for example: the version 3 of the image correspond to the version 4
	// of the Azure Func core Tool 
	version: *#DefaultVersion | string
}

#Image: {

	config: #Config

	docker.#Build & {
		steps: [
			docker.#Pull & {
				source: "barbo69/dagger-azure-cli:\(config.version)"
			},
			docker.#Run & {
				command: {
					name: "az"
					args: ["login"]
				}
			},

			docker.#Run & {
				env: AZ_SUB_ID_TOKEN: config.subscriptionId
				command: {
					name: "sh"
					flags: "-c": "az account set -s $AZ_SUB_ID_TOKEN"
				}
			},
		]
	}
}
