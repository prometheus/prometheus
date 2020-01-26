/*
Package rules provides the ability to retrieve and manage QoS policy rules through the Neutron API.

Example of Listing BandwidthLimitRules

    listOpts := rules.BandwidthLimitRulesListOpts{
        MaxKBps: 3000,
    }

    policyID := "501005fa-3b56-4061-aaca-3f24995112e1"

    allPages, err := rules.ListBandwidthLimitRules(networkClient, policyID, listOpts).AllPages()
    if err != nil {
        panic(err)
    }

    allBandwidthLimitRules, err := rules.ExtractBandwidthLimitRules(allPages)
    if err != nil {
        panic(err)
    }

    for _, bandwidthLimitRule := range allBandwidthLimitRules {
        fmt.Printf("%+v\n", bandwidthLimitRule)
    }

Example of Getting a single BandwidthLimitRule

    policyID := "501005fa-3b56-4061-aaca-3f24995112e1"
    ruleID   := "30a57f4a-336b-4382-8275-d708babd2241"

    rule, err := rules.GetBandwidthLimitRule(networkClient, policyID, ruleID).ExtractBandwidthLimitRule()
    if err != nil {
        panic(err)
    }

    fmt.Printf("Rule: %+v\n", rule)

Example of Creating a single BandwidthLimitRule

    opts := rules.CreateBandwidthLimitRuleOpts{
        MaxKBps:      2000,
        MaxBurstKBps: 200,
    }

    policyID := "501005fa-3b56-4061-aaca-3f24995112e1"

    rule, err := rules.CreateBandwidthLimitRule(networkClient, policyID, opts).ExtractBandwidthLimitRule()
    if err != nil {
        panic(err)
    }

    fmt.Printf("Rule: %+v\n", rule)

Example of Updating a single BandwidthLimitRule

    maxKBps := 500
    maxBurstKBps := 0

    opts := rules.UpdateBandwidthLimitRuleOpts{
        MaxKBps:      &maxKBps,
        MaxBurstKBps: &maxBurstKBps,
    }

    policyID := "501005fa-3b56-4061-aaca-3f24995112e1"
    ruleID   := "30a57f4a-336b-4382-8275-d708babd2241"

    rule, err := rules.UpdateBandwidthLimitRule(networkClient, policyID, ruleID, opts).ExtractBandwidthLimitRule()
    if err != nil {
        panic(err)
    }

    fmt.Printf("Rule: %+v\n", rule)

Example of Deleting a single BandwidthLimitRule

    policyID := "501005fa-3b56-4061-aaca-3f24995112e1"
    ruleID   := "30a57f4a-336b-4382-8275-d708babd2241"

    err := rules.DeleteBandwidthLimitRule(fake.ServiceClient(), "501005fa-3b56-4061-aaca-3f24995112e1", "30a57f4a-336b-4382-8275-d708babd2241").ExtractErr()
    if err != nil {
        panic(err)
    }

Example of Listing DSCP marking rules

    listOpts := rules.DSCPMarkingRulesListOpts{}

    policyID := "501005fa-3b56-4061-aaca-3f24995112e1"

    allPages, err := rules.ListDSCPMarkingRules(networkClient, policyID, listOpts).AllPages()
    if err != nil {
        panic(err)
    }

    allDSCPMarkingRules, err := rules.ExtractDSCPMarkingRules(allPages)
    if err != nil {
        panic(err)
    }

    for _, dscpMarkingRule := range allDSCPMarkingRules {
        fmt.Printf("%+v\n", dscpMarkingRule)
    }

Example of Getting a single DSCPMarkingRule

    policyID := "501005fa-3b56-4061-aaca-3f24995112e1"
    ruleID   := "30a57f4a-336b-4382-8275-d708babd2241"

    rule, err := rules.GetDSCPMarkingRule(networkClient, policyID, ruleID).ExtractDSCPMarkingRule()
    if err != nil {
        panic(err)
    }

    fmt.Printf("Rule: %+v\n", rule)

Example of Creating a single DSCPMarkingRule

    opts := rules.CreateDSCPMarkingRuleOpts{
        DSCPMark: 20,
    }

    policyID := "501005fa-3b56-4061-aaca-3f24995112e1"

    rule, err := rules.CreateDSCPMarkingRule(networkClient, policyID, opts).ExtractDSCPMarkingRule()
    if err != nil {
        panic(err)
    }

    fmt.Printf("Rule: %+v\n", rule)

Example of Updating a single DSCPMarkingRule

    dscpMark := 26

    opts := rules.UpdateDSCPMarkingRuleOpts{
        DSCPMark: &dscpMark,
    }

    policyID := "501005fa-3b56-4061-aaca-3f24995112e1"
    ruleID   := "30a57f4a-336b-4382-8275-d708babd2241"

    rule, err := rules.UpdateDSCPMarkingRule(networkClient, policyID, ruleID, opts).ExtractDSCPMarkingRule()
    if err != nil {
        panic(err)
    }

    fmt.Printf("Rule: %+v\n", rule)

Example of Deleting a single DSCPMarkingRule

    policyID := "501005fa-3b56-4061-aaca-3f24995112e1"
    ruleID   := "30a57f4a-336b-4382-8275-d708babd2241"

    err := rules.DeleteDSCPMarkingRule(fake.ServiceClient(), "501005fa-3b56-4061-aaca-3f24995112e1", "30a57f4a-336b-4382-8275-d708babd2241").ExtractErr()
    if err != nil {
        panic(err)
    }

Example of Listing MinimumBandwidthRules

    listOpts := rules.MinimumBandwidthRulesListOpts{
        MinKBps: 3000,
    }

    policyID := "501005fa-3b56-4061-aaca-3f24995112e1"

    allPages, err := rules.ListMinimumBandwidthRules(networkClient, policyID, listOpts).AllPages()
    if err != nil {
        panic(err)
    }

    allMinimumBandwidthRules, err := rules.ExtractMinimumBandwidthRules(allPages)
    if err != nil {
        panic(err)
    }

    for _, bandwidthLimitRule := range allMinimumBandwidthRules {
        fmt.Printf("%+v\n", bandwidthLimitRule)
    }

Example of Getting a single MinimumBandwidthRule

    policyID := "501005fa-3b56-4061-aaca-3f24995112e1"
    ruleID   := "30a57f4a-336b-4382-8275-d708babd2241"

    rule, err := rules.GetMinimumBandwidthRule(networkClient, policyID, ruleID).ExtractMinimumBandwidthRule()
    if err != nil {
        panic(err)
    }

    fmt.Printf("Rule: %+v\n", rule)

Example of Creating a single MinimumBandwidthRule

    opts := rules.CreateMinimumBandwidthRuleOpts{
        MinKBps: 2000,
    }

    policyID := "501005fa-3b56-4061-aaca-3f24995112e1"

    rule, err := rules.CreateMinimumBandwidthRule(networkClient, policyID, opts).ExtractMinimumBandwidthRule()
    if err != nil {
        panic(err)
    }

    fmt.Printf("Rule: %+v\n", rule)

Example of Updating a single MinimumBandwidthRule

    minKBps := 500

    opts := rules.UpdateMinimumBandwidthRuleOpts{
        MinKBps: &minKBps,
    }

    policyID := "501005fa-3b56-4061-aaca-3f24995112e1"
    ruleID   := "30a57f4a-336b-4382-8275-d708babd2241"

    rule, err := rules.UpdateMinimumBandwidthRule(networkClient, policyID, ruleID, opts).ExtractMinimumBandwidthRule()
    if err != nil {
        panic(err)
    }

    fmt.Printf("Rule: %+v\n", rule)

Example of Deleting a single MinimumBandwidthRule

    policyID := "501005fa-3b56-4061-aaca-3f24995112e1"
    ruleID   := "30a57f4a-336b-4382-8275-d708babd2241"

    err := rules.DeleteMinimumBandwidthRule(fake.ServiceClient(), "501005fa-3b56-4061-aaca-3f24995112e1", "30a57f4a-336b-4382-8275-d708babd2241").ExtractErr()
    if err != nil {
        panic(err)
    }
*/
package rules
