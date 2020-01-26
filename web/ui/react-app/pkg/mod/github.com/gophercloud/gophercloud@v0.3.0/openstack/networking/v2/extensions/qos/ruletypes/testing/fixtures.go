package testing

const (
	ListRuleTypesResponse = `
{
    "rule_types": [
        {
            "type": "bandwidth_limit"
        },
        {
            "type": "dscp_marking"
        },
        {
            "type": "minimum_bandwidth"
        }
    ]
}
`

	GetRuleTypeResponse = `
{
    "rule_type": {
        "drivers": [
            {
                "name": "linuxbridge",
                "supported_parameters": [
                    {
                        "parameter_values": {
                            "start": 0,
                            "end": 2147483647
                        },
                        "parameter_type": "range",
                        "parameter_name": "max_kbps"
                    },
                    {
                        "parameter_values": [
                            "ingress",
                            "egress"
                        ],
                        "parameter_type": "choices",
                        "parameter_name": "direction"
                    },
                    {
                        "parameter_values": {
                            "start": 0,
                            "end": 2147483647
                        },
                        "parameter_type": "range",
                        "parameter_name": "max_burst_kbps"
                    }
                ]
            },
            {
                "name": "openvswitch",
                "supported_parameters": [
                    {
                        "parameter_values": {
                            "start": 0,
                            "end": 2147483647
                        },
                        "parameter_type": "range",
                        "parameter_name": "max_kbps"
                    },
                    {
                        "parameter_values": [
                            "ingress",
                            "egress"
                        ],
                        "parameter_type": "choices",
                        "parameter_name": "direction"
                    },
                    {
                        "parameter_values": {
                            "start": 0,
                            "end": 2147483647
                        },
                        "parameter_type": "range",
                        "parameter_name": "max_burst_kbps"
                    }
                ]
            }
        ],
        "type": "bandwidth_limit"
    }
}
`
)
