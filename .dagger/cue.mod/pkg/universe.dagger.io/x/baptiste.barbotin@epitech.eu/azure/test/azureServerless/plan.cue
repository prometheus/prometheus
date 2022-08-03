package azureServerlessTest

import (
	"dagger.io/dagger"
	"universe.dagger.io/x/baptiste.barbotin@epitech.eu/azure/azure/login"
	"universe.dagger.io/x/baptiste.barbotin@epitech.eu/azure/azureServerless"
)

dagger.#Plan & {

	client: {
		env: AZ_SUB_ID_TOKEN: dagger.#Secret

		filesystem: "./deploy_function": read: contents: dagger.#FS
	}

	actions: deployFunction: azureServerless.#Deploy & {

		source: client.filesystem."./deploy_function".read.contents

		config: azureServerless.#Config & {
			"login": login.#Config & {
				subscriptionId: client.env.AZ_SUB_ID_TOKEN
				version:        "3.0"
			}
			location: "northeurope"
			resourceGroup: name: "daggerrg"
			storage: name:       "daggerst1"
			functionApp: {
				name: "daggerfa1"
				args: ["--runtime", "node", "--runtime-version", "14"]
			}
			publishFunction: args: ["--javascript"]

		}
	}
}
