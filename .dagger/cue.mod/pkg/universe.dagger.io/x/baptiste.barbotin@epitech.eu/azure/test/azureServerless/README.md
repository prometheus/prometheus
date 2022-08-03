# AzureServeless Example

You can have a look at the [plan.cue](./plan.cue) file which contains the example to deploy a serverless function with Azure.

If you want to deploy this serveless function, you need to set an AZ_SUB_ID_TOKEN environnment variable with your Azure Subscription id that you can get with the command:

```shell
az account list
```

To set your environement variable you can copy the content of the example.envrc file and add it in a .envrc file to use direnv or manually set it.

Once you've setup everything, you can simply run:

```shell
dagger project init
dagger project update
dagger do deployFunction
```
It will deploy a cloud function sending back HelloWorld when you make a request on the url.