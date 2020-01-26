package testing

import (
	"time"

	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/qos/policies"
)

const GetPortResponse = `
{
    "port": {
        "id": "65c0ee9f-d634-4522-8954-51021b570b0d",
        "qos_policy_id": "591e0597-39a6-4665-8149-2111d8de9a08"
    }
}
`

const CreatePortRequest = `
{
    "port": {
        "network_id": "a87cc70a-3e15-4acf-8205-9b711a3531b7",
        "qos_policy_id": "591e0597-39a6-4665-8149-2111d8de9a08"
    }
}
`

const CreatePortResponse = `
{
    "port": {
        "network_id": "a87cc70a-3e15-4acf-8205-9b711a3531b7",
        "tenant_id": "d6700c0c9ffa4f1cb322cd4a1f3906fa",
        "id": "65c0ee9f-d634-4522-8954-51021b570b0d",
        "qos_policy_id": "591e0597-39a6-4665-8149-2111d8de9a08"
    }
}
`

const UpdatePortWithPolicyRequest = `
{
    "port": {
        "qos_policy_id": "591e0597-39a6-4665-8149-2111d8de9a08"
    }
}
`

const UpdatePortWithPolicyResponse = `
{
    "port": {
        "network_id": "a87cc70a-3e15-4acf-8205-9b711a3531b7",
        "tenant_id": "d6700c0c9ffa4f1cb322cd4a1f3906fa",
        "id": "65c0ee9f-d634-4522-8954-51021b570b0d",
        "qos_policy_id": "591e0597-39a6-4665-8149-2111d8de9a08"
    }
}
`

const UpdatePortWithoutPolicyRequest = `
{
    "port": {
        "qos_policy_id": null
    }
}
`

const UpdatePortWithoutPolicyResponse = `
{
    "port": {
        "network_id": "a87cc70a-3e15-4acf-8205-9b711a3531b7",
        "tenant_id": "d6700c0c9ffa4f1cb322cd4a1f3906fa",
        "id": "65c0ee9f-d634-4522-8954-51021b570b0d",
        "qos_policy_id": ""
    }
}
`

const GetNetworkResponse = `
{
    "network": {
        "id": "65c0ee9f-d634-4522-8954-51021b570b0d",
        "qos_policy_id": "591e0597-39a6-4665-8149-2111d8de9a08"
    }
}
`

const CreateNetworkRequest = `
{
    "network": {
        "name": "private",
        "qos_policy_id": "591e0597-39a6-4665-8149-2111d8de9a08"
    }
}
`

const CreateNetworkResponse = `
{
    "network": {
        "tenant_id": "4fd44f30292945e481c7b8a0c8908869",
        "id": "65c0ee9f-d634-4522-8954-51021b570b0d",
        "qos_policy_id": "591e0597-39a6-4665-8149-2111d8de9a08"
    }
}
`

const UpdateNetworkWithPolicyRequest = `
{
    "network": {
        "name": "updated",
        "qos_policy_id": "591e0597-39a6-4665-8149-2111d8de9a08"
    }
}
`

const UpdateNetworkWithPolicyResponse = `
{
    "network": {
        "tenant_id": "4fd44f30292945e481c7b8a0c8908869",
        "id": "65c0ee9f-d634-4522-8954-51021b570b0d",
        "name": "updated",
        "qos_policy_id": "591e0597-39a6-4665-8149-2111d8de9a08"
    }
}
`

const UpdateNetworkWithoutPolicyRequest = `
{
    "network": {
        "qos_policy_id": null
    }
}
`

const UpdateNetworkWithoutPolicyResponse = `
{
    "network": {
        "tenant_id": "4fd44f30292945e481c7b8a0c8908869",
        "id": "65c0ee9f-d634-4522-8954-51021b570b0d",
        "qos_policy_id": ""
    }
}
`

const ListPoliciesResponse = `
{
    "policies": [
        {
            "name": "bw-limiter",
            "tags": [],
            "rules": [
                {
                    "max_kbps": 3000,
                    "direction": "egress",
                    "qos_policy_id": "d6ae28ce-fcb5-4180-aa62-d260a27e09ae",
                    "type": "bandwidth_limit",
                    "id": "30a57f4a-336b-4382-8275-d708babd2241",
                    "max_burst_kbps": 300
                }
            ],
            "tenant_id": "a77cbe0998374aed9a6798ad6c61677e",
            "created_at": "2019-05-19T11:17:50Z",
            "updated_at": "2019-05-19T11:17:57Z",
            "is_default": false,
            "revision_number": 1,
            "shared": false,
            "project_id": "a77cbe0998374aed9a6798ad6c61677e",
            "id": "d6ae28ce-fcb5-4180-aa62-d260a27e09ae",
            "description": ""
        },
        {
            "name": "no-rules",
            "tags": [],
            "rules": [],
            "tenant_id": "a77cbe0998374aed9a6798ad6c61677e",
            "created_at": "2019-06-01T10:38:58Z",
            "updated_at": "2019-06-01T10:38:58Z",
            "is_default": false,
            "revision_number": 0,
            "shared": false,
            "project_id": "a77cbe0998374aed9a6798ad6c61677e",
            "id": "d6e7c2fe-24dc-43be-a088-24b47d4f8f88",
            "description": ""
        }
    ]
}
`

var Policy1 = policies.Policy{
	Name: "bw-limiter",
	Rules: []map[string]interface{}{
		{
			"type":           "bandwidth_limit",
			"max_kbps":       float64(3000),
			"direction":      "egress",
			"qos_policy_id":  "d6ae28ce-fcb5-4180-aa62-d260a27e09ae",
			"max_burst_kbps": float64(300),
			"id":             "30a57f4a-336b-4382-8275-d708babd2241",
		},
	},
	Tags:           []string{},
	TenantID:       "a77cbe0998374aed9a6798ad6c61677e",
	CreatedAt:      time.Date(2019, 5, 19, 11, 17, 50, 0, time.UTC),
	UpdatedAt:      time.Date(2019, 5, 19, 11, 17, 57, 0, time.UTC),
	RevisionNumber: 1,
	ProjectID:      "a77cbe0998374aed9a6798ad6c61677e",
	ID:             "d6ae28ce-fcb5-4180-aa62-d260a27e09ae",
}

var Policy2 = policies.Policy{
	Name:           "no-rules",
	Tags:           []string{},
	Rules:          []map[string]interface{}{},
	TenantID:       "a77cbe0998374aed9a6798ad6c61677e",
	CreatedAt:      time.Date(2019, 6, 1, 10, 38, 58, 0, time.UTC),
	UpdatedAt:      time.Date(2019, 6, 1, 10, 38, 58, 0, time.UTC),
	RevisionNumber: 0,
	ProjectID:      "a77cbe0998374aed9a6798ad6c61677e",
	ID:             "d6e7c2fe-24dc-43be-a088-24b47d4f8f88",
}

const GetPolicyResponse = `
{
    "policy": {
        "name": "bw-limiter",
        "tags": [],
        "rules": [
            {
                "max_kbps": 3000,
                "direction": "egress",
                "qos_policy_id": "d6ae28ce-fcb5-4180-aa62-d260a27e09ae",
                "type": "bandwidth_limit",
                "id": "30a57f4a-336b-4382-8275-d708babd2241",
                "max_burst_kbps": 300
            }
        ],
        "tenant_id": "a77cbe0998374aed9a6798ad6c61677e",
        "created_at": "2019-05-19T11:17:50Z",
        "updated_at": "2019-05-19T11:17:57Z",
        "is_default": false,
        "revision_number": 1,
        "shared": false,
        "project_id": "a77cbe0998374aed9a6798ad6c61677e",
        "id": "d6ae28ce-fcb5-4180-aa62-d260a27e09ae",
        "description": ""
    }
}
`

const CreatePolicyRequest = `
{
    "policy": {
        "name": "shared-default-policy",
        "is_default": true,
        "shared": true,
        "description": "use-me"
    }
}
`

const CreatePolicyResponse = `
{
    "policy": {
        "name": "shared-default-policy",
        "tags": [],
        "rules": [],
        "tenant_id": "a77cbe0998374aed9a6798ad6c61677e",
        "created_at": "2019-05-19T11:17:50Z",
        "updated_at": "2019-05-19T11:17:57Z",
        "is_default": true,
        "revision_number": 0,
        "shared": true,
        "project_id": "a77cbe0998374aed9a6798ad6c61677e",
        "id": "d6ae28ce-fcb5-4180-aa62-d260a27e09ae",
        "description": "use-me"
    }
}
`

const UpdatePolicyRequest = `
{
    "policy": {
        "name": "new-name",
        "shared": true,
        "description": ""
    }
}
`

const UpdatePolicyResponse = `
{
    "policy": {
        "name": "new-name",
        "tags": [],
        "rules": [],
        "tenant_id": "a77cbe0998374aed9a6798ad6c61677e",
        "created_at": "2019-05-19T11:17:50Z",
        "updated_at": "2019-06-01T13:17:57Z",
        "is_default": false,
        "revision_number": 1,
        "shared": true,
        "project_id": "a77cbe0998374aed9a6798ad6c61677e",
        "id": "d6ae28ce-fcb5-4180-aa62-d260a27e09ae",
        "description": ""
    }
}
`
