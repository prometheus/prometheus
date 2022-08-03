package azureServerless

import (
	"dagger.io/dagger"
	azLogin "universe.dagger.io/x/baptiste.barbotin@epitech.eu/azure/azure/login"
	"universe.dagger.io/x/baptiste.barbotin@epitech.eu/azure/azure/resourceGroup"
	"universe.dagger.io/x/baptiste.barbotin@epitech.eu/azure/azure/storage/account"
	"universe.dagger.io/x/baptiste.barbotin@epitech.eu/azure/azure/functionApp"
	"universe.dagger.io/x/baptiste.barbotin@epitech.eu/azure/azureFuncCoreTool"
)

#Config: {

	// Azure login
	login: azLogin.#Config

	// Azure server location
	location: string

	// Azure ressource group name
	resourceGroup: name: string

	// Azure storage name
	storage: name: string

	// Azure functionApp
	functionApp: {
		name:    string
		version: string | *"4"
		args:    [...string] | *[]
	}

	// Azure publish function args
	publishFunction: {
		args:  [...string] | *[]
		sleep: string
	}

	// Sleep time default value
	if publishFunction.sleep == _|_ {
		publishFunction: sleep: "30"
	}
}

#Deploy: {

	// Source directory
	source: dagger.#FS

	config: #Config

	login: azLogin.#Image & {
		"config": config.login
	}

	createRessourceGroup: resourceGroup.#Create & {
		image:    login.output
		name:     config.resourceGroup.name
		location: config.location
	}
	createStorageAccount: account.#Create & {
		image: createRessourceGroup.output
		resourceGroup: name: config.resourceGroup.name
		name:     config.storage.name
		location: config.location
	}
	createFunctionApp: functionApp.#Create & {
		image: createStorageAccount.output
		resourceGroup: name: config.resourceGroup.name
		storage: name:       config.storage.name
		name:     config.functionApp.name
		location: config.location
		version:  config.functionApp.version
		args:     config.functionApp.args
	}
	publishFunction: azureFuncCoreTool.#Publish & {
		image:    createFunctionApp.output
		name:     config.functionApp.name
		"source": source
		args:     config.publishFunction.args
		sleep:    config.publishFunction.sleep
	}
}
