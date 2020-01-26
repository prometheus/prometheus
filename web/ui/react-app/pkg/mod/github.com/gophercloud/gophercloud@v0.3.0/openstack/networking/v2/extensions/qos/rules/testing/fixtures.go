package testing

// BandwidthLimitRulesListResult represents a raw result of a List call to BandwidthLimitRules.
const BandwidthLimitRulesListResult = `
{
    "bandwidth_limit_rules": [
        {
            "max_kbps": 3000,
            "direction": "egress",
            "id": "30a57f4a-336b-4382-8275-d708babd2241",
            "max_burst_kbps": 300
        }
    ]
}
`

// BandwidthLimitRulesGetResult represents a raw result of a Get call to a specific BandwidthLimitRule.
const BandwidthLimitRulesGetResult = `
{
    "bandwidth_limit_rule": {
        "max_kbps": 3000,
        "direction": "egress",
        "id": "30a57f4a-336b-4382-8275-d708babd2241",
        "max_burst_kbps": 300
    }
}
`

// BandwidthLimitRulesCreateRequest represents a raw body of a Create BandwidthLimitRule call.
const BandwidthLimitRulesCreateRequest = `
{
    "bandwidth_limit_rule": {
        "max_kbps": 2000,
        "max_burst_kbps": 200
    }
}
`

// BandwidthLimitRulesCreateResult represents a raw result of a Create BandwidthLimitRule call.
const BandwidthLimitRulesCreateResult = `
{
    "bandwidth_limit_rule": {
        "max_kbps": 2000,
        "id": "30a57f4a-336b-4382-8275-d708babd2241",
        "max_burst_kbps": 200
    }
}
`

// BandwidthLimitRulesUpdateRequest represents a raw body of a Update BandwidthLimitRule call.
const BandwidthLimitRulesUpdateRequest = `
{
    "bandwidth_limit_rule": {
        "max_kbps": 500,
        "max_burst_kbps": 0
    }
}
`

// BandwidthLimitRulesUpdateResult represents a raw result of a Update BandwidthLimitRule call.
const BandwidthLimitRulesUpdateResult = `
{
    "bandwidth_limit_rule": {
        "max_kbps": 500,
        "id": "30a57f4a-336b-4382-8275-d708babd2241",
        "max_burst_kbps": 0
    }
}
`

// DSCPMarkingRulesListResult represents a raw result of a List call to DSCPMarkingRules.
const DSCPMarkingRulesListResult = `
{
    "dscp_marking_rules": [
        {
            "id": "30a57f4a-336b-4382-8275-d708babd2241",
            "dscp_mark": 20
        }
    ]
}
`

// DSCPMarkingRuleGetResult represents a raw result of a Get DSCPMarkingRule call.
const DSCPMarkingRuleGetResult = `
{
    "dscp_marking_rule": {
        "id": "30a57f4a-336b-4382-8275-d708babd2241",
        "dscp_mark": 26
    }
}
`

// DSCPMarkingRuleCreateRequest represents a raw body of a Create DSCPMarkingRule call.
const DSCPMarkingRuleCreateRequest = `
{
    "dscp_marking_rule": {
        "dscp_mark": 20
    }
}
`

// DSCPMarkingRuleCreateResult represents a raw result of a Update DSCPMarkingRule call.
const DSCPMarkingRuleCreateResult = `
{
    "dscp_marking_rule": {
        "id": "30a57f4a-336b-4382-8275-d708babd2241",
        "dscp_mark": 20
    }
}
`

// DSCPMarkingRuleUpdateRequest represents a raw body of a Update DSCPMarkingRule call.
const DSCPMarkingRuleUpdateRequest = `
{
    "dscp_marking_rule": {
        "dscp_mark": 26
    }
}
`

// DSCPMarkingRuleUpdateResult represents a raw result of a Update DSCPMarkingRule call.
const DSCPMarkingRuleUpdateResult = `
{
    "dscp_marking_rule": {
        "id": "30a57f4a-336b-4382-8275-d708babd2241",
        "dscp_mark": 26
    }
}
`

// MinimumBandwidthRulesListResult represents a raw result of a List call to MinimumBandwidthRules.
const MinimumBandwidthRulesListResult = `
{
    "minimum_bandwidth_rules": [
        {
            "min_kbps": 3000,
            "direction": "egress",
            "id": "30a57f4a-336b-4382-8275-d708babd2241"
        }
    ]
}
`

// MinimumBandwidthRulesGetResult represents a raw result of a Get call to a specific MinimumBandwidthRule.
const MinimumBandwidthRulesGetResult = `
{
    "minimum_bandwidth_rule": {
        "min_kbps": 3000,
        "direction": "egress",
        "id": "30a57f4a-336b-4382-8275-d708babd2241"
    }
}
`

// MinimumBandwidthRulesCreateRequest represents a raw body of a Create MinimumBandwidthRule call.
const MinimumBandwidthRulesCreateRequest = `
{
    "minimum_bandwidth_rule": {
        "min_kbps": 2000
    }
}
`

// MinimumBandwidthRulesCreateResult represents a raw result of a Create MinimumBandwidthRule call.
const MinimumBandwidthRulesCreateResult = `
{
    "minimum_bandwidth_rule": {
        "min_kbps": 2000,
        "id": "30a57f4a-336b-4382-8275-d708babd2241"
    }
}
`

// MinimumBandwidthRulesUpdateRequest represents a raw body of a Update MinimumBandwidthRule call.
const MinimumBandwidthRulesUpdateRequest = `
{
    "minimum_bandwidth_rule": {
        "min_kbps": 500
    }
}
`

// MinimumBandwidthRulesUpdateResult represents a raw result of a Update MinimumBandwidthRule call.
const MinimumBandwidthRulesUpdateResult = `
{
    "minimum_bandwidth_rule": {
        "min_kbps": 500,
        "id": "30a57f4a-336b-4382-8275-d708babd2241"
    }
}
`
