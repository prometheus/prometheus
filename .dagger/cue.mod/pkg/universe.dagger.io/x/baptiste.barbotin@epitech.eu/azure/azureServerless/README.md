# AzureServerless

This package provides declarations to interact with Azure and Azure Func Core Tool for creating and uploading an Azure Serverless function.

The following are available:
- [Config](#config)
- [Deploy](#deploy)


## Config

This is the config part of the package. It allows us to complete all the informations that we need to deploy an Azure serverless function.

It is available in the [deployment.cue](./deployment.cue) file.

## Deploy

This is the deployment part of the package. It allows us to deploy our Azure serverless function.

It is available in the [deployment.cue](./deployment.cue) file.

This package depends on the `Azure` and `AzureFuncCoreTool` packages. You can check the documentation here: [Azure](./azure/README.md), [AzureFuncCoreTool](./azureFuncCoreTool/README.md).

## Examples

For examples of how to use this package, you can check the examples [right here](../test/azureServerless/README.md)