/*
Package ruletypes contains functionality for working with Neutron 'quality of service' rule-type resources.

Example of Listing QoS rule types

	page, err := ruletypes.ListRuleTypes(client).AllPages()
	if err != nil {
		return
	}

	rules, err := ruletypes.ExtractRuleTypes(page)
	if err != nil {
		return
	}

	fmt.Printf("%v <- Rule Types\n", rules)

Example of Getting a single QoS rule type by it's name

    ruleTypeName := "bandwidth_limit"

    ruleType, err := ruletypes.Get(networkClient, ruleTypeName).Extract()
    if err != nil {
        panic(err)
    }

    fmt.Printf("%+v\n", ruleTypeName)
*/
package ruletypes
