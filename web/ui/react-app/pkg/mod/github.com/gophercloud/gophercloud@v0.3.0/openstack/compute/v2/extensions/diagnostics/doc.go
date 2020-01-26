/*
Package diagnostics returns details about a nova instance diagnostics

Example of Show Diagnostics

	diags, err := diagnostics.Get(computeClient, serverId).Extract()
	if err != nil {
		panic(err)
	}

	fmt.Printf("%+v\n", diags)

*/
package diagnostics
