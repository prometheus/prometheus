// Copyright 2017 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package openstack

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// SDMock is the interface for the OpenStack mock.
type SDMock struct {
	t      *testing.T
	Server *httptest.Server
	Mux    *http.ServeMux
}

// NewSDMock returns a new SDMock.
func NewSDMock(t *testing.T) *SDMock {
	return &SDMock{
		t: t,
	}
}

// Endpoint returns the URI to the mock server.
func (m *SDMock) Endpoint() string {
	return m.Server.URL + "/"
}

// Setup creates the mock server.
func (m *SDMock) Setup() {
	m.Mux = http.NewServeMux()
	m.Server = httptest.NewServer(m.Mux)
	m.t.Cleanup(m.Server.Close)
}

const tokenID = "cbc36478b0bd8e67e89469c7749d4127"

func testMethod(t *testing.T, r *http.Request, expected string) {
	require.Equal(t, expected, r.Method, "Unexpected request method.")
}

func testHeader(t *testing.T, r *http.Request, header, expected string) {
	t.Helper()
	actual := r.Header.Get(header)
	require.Equal(t, expected, actual, "Unexpected value for request header %s.", header)
}

// HandleVersionsSuccessfully mocks version call.
func (m *SDMock) HandleVersionsSuccessfully() {
	m.Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `
                        {
                                "versions": {
                                        "values": [
                                                {
                                                        "status": "stable",
                                                        "id": "v3.0",
                                                        "links": [
                                                                { "href": "%s", "rel": "self" }
                                                        ]
                                                },
                                                {
                                                        "status": "stable",
                                                        "id": "v2.0",
                                                        "links": [
                                                                { "href": "%s", "rel": "self" }
                                                        ]
                                                }
                                        ]
                                }
                        }
                `, m.Endpoint()+"v3/", m.Endpoint()+"v2.0/")
	})
}

// HandleAuthSuccessfully mocks auth call.
func (m *SDMock) HandleAuthSuccessfully() {
	m.Mux.HandleFunc("/v3/auth/tokens", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("X-Subject-Token", tokenID)

		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `
	{
    "token": {
        "audit_ids": ["VcxU2JYqT8OzfUVvrjEITQ", "qNUTIJntTzO1-XUk5STybw"],
        "catalog": [
            {
                "endpoints": [
                    {
                        "id": "39dc322ce86c4111b4f06c2eeae0841b",
                        "interface": "public",
                        "region": "RegionOne",
                        "url": "http://localhost:5000"
                    },
                    {
                        "id": "ec642f27474842e78bf059f6c48f4e99",
                        "interface": "internal",
                        "region": "RegionOne",
                        "url": "http://localhost:5000"
                    },
                    {
                        "id": "c609fc430175452290b62a4242e8a7e8",
                        "interface": "admin",
                        "region": "RegionOne",
                        "url": "http://localhost:35357"
                    }
                ],
                "id": "4363ae44bdf34a3981fde3b823cb9aa2",
                "type": "identity",
                "name": "keystone"
            },
            {
                "endpoints": [
                    {
                        "id": "e2ffee808abc4a60916715b1d4b489dd",
                        "interface": "public",
                        "region": "RegionOne",
                        "region_id": "RegionOne",
                        "url": "%s"
                    }
                ],
                "id": "b7f2a5b1a019459cb956e43a8cb41e31",
                "type": "compute"
            },
            {
                "endpoints": [
                    {
                        "id": "39dc322ce86c4111b4f06c2eeae0841b",
                        "interface": "public",
                        "region": "RegionOne",
                        "region_id": "RegionOne",
                        "url": "%s"
                    }
                ],
                "id": "c609fc430175452290b62a4242e8a7e8",
                "type": "network"
        }
        ],
        "expires_at": "2013-02-27T18:30:59.999999Z",
        "is_domain": false,
        "issued_at": "2013-02-27T16:30:59.999999Z",
        "methods": [
            "password"
        ],
        "project": {
            "domain": {
                "id": "1789d1",
                "name": "example.com"
            },
            "id": "263fd9",
            "name": "project-x"
        },
        "roles": [
            {
                "id": "76e72a",
                "name": "admin"
            },
            {
                "id": "f4f392",
                "name": "member"
            }
        ],
        "user": {
            "domain": {
                "id": "1789d1",
                "name": "example.com"
            },
            "id": "0ca8f6",
            "name": "Joe",
            "password_expires_at": "2016-11-06T15:32:17.000000"
        }
    }
}
	`, m.Endpoint(), m.Endpoint())
	})
}

const hypervisorListBody = `
{
    "hypervisors": [
        {
            "status": "enabled",
            "service": {
                "host": "nc14.cloud.com",
                "disabled_reason": null,
                "id": 16
            },
            "vcpus_used": 18,
            "hypervisor_type": "QEMU",
            "local_gb_used": 84,
            "vcpus": 24,
            "hypervisor_hostname": "nc14.cloud.com",
            "memory_mb_used": 24064,
            "memory_mb": 96484,
            "current_workload": 1,
            "state": "up",
            "host_ip": "172.16.70.14",
            "cpu_info": "{\"vendor\": \"Intel\", \"model\": \"IvyBridge\", \"arch\": \"x86_64\", \"features\": [\"pge\", \"avx\", \"clflush\", \"sep\", \"syscall\", \"vme\", \"dtes64\", \"msr\", \"fsgsbase\", \"xsave\", \"vmx\", \"erms\", \"xtpr\", \"cmov\", \"smep\", \"ssse3\", \"est\", \"pat\", \"monitor\", \"smx\", \"pbe\", \"lm\", \"tsc\", \"nx\", \"fxsr\", \"tm\", \"sse4.1\", \"pae\", \"sse4.2\", \"pclmuldq\", \"acpi\", \"tsc-deadline\", \"mmx\", \"osxsave\", \"cx8\", \"mce\", \"de\", \"tm2\", \"ht\", \"dca\", \"lahf_lm\", \"popcnt\", \"mca\", \"pdpe1gb\", \"apic\", \"sse\", \"f16c\", \"pse\", \"ds\", \"invtsc\", \"pni\", \"rdtscp\", \"aes\", \"sse2\", \"ss\", \"ds_cpl\", \"pcid\", \"fpu\", \"cx16\", \"pse36\", \"mtrr\", \"pdcm\", \"rdrand\", \"x2apic\"], \"topology\": {\"cores\": 6, \"cells\": 2, \"threads\": 2, \"sockets\": 1}}",
            "running_vms": 10,
            "free_disk_gb": 315,
            "hypervisor_version": 2003000,
            "disk_available_least": 304,
            "local_gb": 399,
            "free_ram_mb": 72420,
            "id": 1
        },
        {
            "status": "enabled",
            "service": {
                "host": "cc13.cloud.com",
                "disabled_reason": null,
                "id": 17
            },
            "vcpus_used": 1,
            "hypervisor_type": "QEMU",
            "local_gb_used": 20,
            "vcpus": 24,
            "hypervisor_hostname": "cc13.cloud.com",
            "memory_mb_used": 2560,
            "memory_mb": 96484,
            "current_workload": 0,
            "state": "up",
            "host_ip": "172.16.70.13",
            "cpu_info": "{\"vendor\": \"Intel\", \"model\": \"IvyBridge\", \"arch\": \"x86_64\", \"features\": [\"pge\", \"avx\", \"clflush\", \"sep\", \"syscall\", \"vme\", \"dtes64\", \"msr\", \"fsgsbase\", \"xsave\", \"vmx\", \"erms\", \"xtpr\", \"cmov\", \"smep\", \"ssse3\", \"est\", \"pat\", \"monitor\", \"smx\", \"pbe\", \"lm\", \"tsc\", \"nx\", \"fxsr\", \"tm\", \"sse4.1\", \"pae\", \"sse4.2\", \"pclmuldq\", \"acpi\", \"tsc-deadline\", \"mmx\", \"osxsave\", \"cx8\", \"mce\", \"de\", \"tm2\", \"ht\", \"dca\", \"lahf_lm\", \"popcnt\", \"mca\", \"pdpe1gb\", \"apic\", \"sse\", \"f16c\", \"pse\", \"ds\", \"invtsc\", \"pni\", \"rdtscp\", \"aes\", \"sse2\", \"ss\", \"ds_cpl\", \"pcid\", \"fpu\", \"cx16\", \"pse36\", \"mtrr\", \"pdcm\", \"rdrand\", \"x2apic\"], \"topology\": {\"cores\": 6, \"cells\": 2, \"threads\": 2, \"sockets\": 1}}",
            "running_vms": 0,
            "free_disk_gb": 379,
            "hypervisor_version": 2003000,
            "disk_available_least": 384,
            "local_gb": 399,
            "free_ram_mb": 93924,
            "id": 721
        }
    ]
}`

// HandleHypervisorListSuccessfully mocks os-hypervisors detail call.
func (m *SDMock) HandleHypervisorListSuccessfully() {
	m.Mux.HandleFunc("/os-hypervisors/detail", func(w http.ResponseWriter, r *http.Request) {
		testMethod(m.t, r, http.MethodGet)
		testHeader(m.t, r, "X-Auth-Token", tokenID)

		w.Header().Add("Content-Type", "application/json")
		fmt.Fprint(w, hypervisorListBody)
	})
}

const serverListBody = `
{
	"servers": [
		{
			"status": "ERROR",
			"updated": "2014-09-25T13:10:10Z",
			"hostId": "29d3c8c896a45aa4c34e52247875d7fefc3d94bbcc9f622b5d204362",
			"OS-EXT-SRV-ATTR:host": "devstack",
			"addresses": {},
			"links": [
				{
					"href": "http://104.130.131.164:8774/v2/fcad67a6189847c4aecfa3c81a05783b/servers/af9bcad9-3c87-477d-9347-b291eabf480e",
					"rel": "self"
				},
				{
					"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/servers/af9bcad9-3c87-477d-9347-b291eabf480e",
					"rel": "bookmark"
				}
			],
			"key_name": null,
			"image": {
				"id": "f90f6034-2570-4974-8351-6b49732ef2eb",
				"links": [
					{
						"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/images/f90f6034-2570-4974-8351-6b49732ef2eb",
						"rel": "bookmark"
					}
				]
			},
			"OS-EXT-STS:task_state": null,
			"OS-EXT-STS:vm_state": "error",
			"OS-EXT-SRV-ATTR:instance_name": "instance-00000010",
			"OS-SRV-USG:launched_at": "2014-09-25T13:10:10.000000",
			"OS-EXT-SRV-ATTR:hypervisor_hostname": "devstack",
			"flavor": {
				"id": "1",
				"links": [
					{
						"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/flavors/1",
						"rel": "bookmark"
					}
				]
			},
			"id": "af9bcad9-3c87-477d-9347-b291eabf480e",
			"security_groups": [
				{
					"name": "default"
				}
			],
			"OS-SRV-USG:terminated_at": null,
			"OS-EXT-AZ:availability_zone": "nova",
			"user_id": "9349aff8be7545ac9d2f1d00999a23cd",
			"name": "herp2",
			"created": "2014-09-25T13:10:02Z",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"OS-DCF:diskConfig": "MANUAL",
			"os-extended-volumes:volumes_attached": [],
			"accessIPv4": "",
			"accessIPv6": "",
			"progress": 0,
			"OS-EXT-STS:power_state": 1,
			"config_drive": "",
			"metadata": {}
		},
		{
			"status": "ACTIVE",
			"updated": "2014-09-25T13:10:10Z",
			"hostId": "29d3c8c896a45aa4c34e52247875d7fefc3d94bbcc9f622b5d204362",
			"OS-EXT-SRV-ATTR:host": "devstack",
			"addresses": {
				"private": [
					{
						"OS-EXT-IPS-MAC:mac_addr": "fa:16:3e:7c:1b:2b",
						"version": 4,
						"addr": "10.0.0.32",
						"OS-EXT-IPS:type": "fixed"
					},
					{
						"version": 4,
						"addr": "10.10.10.2",
						"OS-EXT-IPS:type": "floating"
					}
				]
			},
			"links": [
				{
					"href": "http://104.130.131.164:8774/v2/fcad67a6189847c4aecfa3c81a05783b/servers/ef079b0c-e610-4dfb-b1aa-b49f07ac48e5",
					"rel": "self"
				},
				{
					"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/servers/ef079b0c-e610-4dfb-b1aa-b49f07ac48e5",
					"rel": "bookmark"
				}
			],
			"key_name": null,
			"image": {
				"id": "f90f6034-2570-4974-8351-6b49732ef2eb",
				"links": [
					{
						"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/images/f90f6034-2570-4974-8351-6b49732ef2eb",
						"rel": "bookmark"
					}
				]
			},
			"OS-EXT-STS:task_state": null,
			"OS-EXT-STS:vm_state": "active",
			"OS-EXT-SRV-ATTR:instance_name": "instance-0000001e",
			"OS-SRV-USG:launched_at": "2014-09-25T13:10:10.000000",
			"OS-EXT-SRV-ATTR:hypervisor_hostname": "devstack",
			"flavor": {
				"id": "1",
				"links": [
					{
						"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/flavors/1",
						"rel": "bookmark"
					}
				]
			},
			"id": "ef079b0c-e610-4dfb-b1aa-b49f07ac48e5",
			"security_groups": [
				{
					"name": "default"
				}
			],
			"OS-SRV-USG:terminated_at": null,
			"OS-EXT-AZ:availability_zone": "nova",
			"user_id": "9349aff8be7545ac9d2f1d00999a23cd",
			"name": "herp",
			"created": "2014-09-25T13:10:02Z",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"OS-DCF:diskConfig": "MANUAL",
			"os-extended-volumes:volumes_attached": [],
			"accessIPv4": "",
			"accessIPv6": "",
			"progress": 0,
			"OS-EXT-STS:power_state": 1,
			"config_drive": "",
			"metadata": {}
		},
		{
			"status": "ACTIVE",
			"updated": "2014-09-25T13:04:49Z",
			"hostId": "29d3c8c896a45aa4c34e52247875d7fefc3d94bbcc9f622b5d204362",
			"OS-EXT-SRV-ATTR:host": "devstack",
			"addresses": {
				"private": [
					{
						"OS-EXT-IPS-MAC:mac_addr": "fa:16:3e:9e:89:be",
						"version": 4,
						"addr": "10.0.0.31",
						"OS-EXT-IPS:type": "fixed"
					}
				]
			},
			"links": [
				{
					"href": "http://104.130.131.164:8774/v2/fcad67a6189847c4aecfa3c81a05783b/servers/9e5476bd-a4ec-4653-93d6-72c93aa682ba",
					"rel": "self"
				},
				{
					"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/servers/9e5476bd-a4ec-4653-93d6-72c93aa682ba",
					"rel": "bookmark"
				}
			],
			"key_name": null,
			"image": {
				"id": "f90f6034-2570-4974-8351-6b49732ef2eb",
				"links": [
					{
						"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/images/f90f6034-2570-4974-8351-6b49732ef2eb",
						"rel": "bookmark"
					}
				]
			},
			"OS-EXT-STS:task_state": null,
			"OS-EXT-STS:vm_state": "active",
			"OS-EXT-SRV-ATTR:instance_name": "instance-0000001d",
			"OS-SRV-USG:launched_at": "2014-09-25T13:04:49.000000",
			"OS-EXT-SRV-ATTR:hypervisor_hostname": "devstack",
			"flavor": {
				"vcpus": 2,
				"ram": 4096,
				"disk": 0,
				"ephemeral": 0,
				"swap": 0,
				"original_name": "m1.medium",
				"extra_specs": {
					"aggregate_instance_extra_specs:general": "true",
					"hw:mem_page_size": "large",
					"hw:vif_multiqueue_enabled": "true"
				}
			},
			"id": "9e5476bd-a4ec-4653-93d6-72c93aa682ba",
			"security_groups": [
				{
					"name": "default"
				}
			],
			"OS-SRV-USG:terminated_at": null,
			"OS-EXT-AZ:availability_zone": "nova",
			"user_id": "9349aff8be7545ac9d2f1d00999a23cd",
			"name": "derp",
			"created": "2014-09-25T13:04:41Z",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"OS-DCF:diskConfig": "MANUAL",
			"os-extended-volumes:volumes_attached": [],
			"accessIPv4": "",
			"accessIPv6": "",
			"progress": 0,
			"OS-EXT-STS:power_state": 1,
			"config_drive": "",
			"metadata": {}
		},
		{
		"status": "ACTIVE",
		"updated": "2014-09-25T13:04:49Z",
		"hostId": "29d3c8c896a45aa4c34e52247875d7fefc3d94bbcc9f622b5d204362",
		"OS-EXT-SRV-ATTR:host": "devstack",
		"addresses": {
			"private": [
				{
					"version": 4,
					"addr": "10.0.0.33",
					"OS-EXT-IPS:type": "fixed"
				},
				{
					"version": 4,
					"addr": "10.0.0.34",
					"OS-EXT-IPS:type": "fixed"
				},
				{
					"version": 4,
					"addr": "10.10.10.4",
					"OS-EXT-IPS:type": "floating"
				}
			]
		},
		"links": [
			{
				"href": "http://104.130.131.164:8774/v2/fcad67a6189847c4aecfa3c81a05783b/servers/9e5476bd-a4ec-4653-93d6-72c93aa682ba",
				"rel": "self"
			},
			{
				"href": "http://104.130.131.164:8774/fcad67a6189847c4aecfa3c81a05783b/servers/9e5476bd-a4ec-4653-93d6-72c93aa682ba",
				"rel": "bookmark"
			}
		],
		"key_name": null,
		"image": "",
		"OS-EXT-STS:task_state": null,
		"OS-EXT-STS:vm_state": "active",
		"OS-EXT-SRV-ATTR:instance_name": "instance-0000001d",
		"OS-SRV-USG:launched_at": "2014-09-25T13:04:49.000000",
		"OS-EXT-SRV-ATTR:hypervisor_hostname": "devstack",
		"flavor": {
			"vcpus": 2,
			"ram": 4096,
			"disk": 0,
			"ephemeral": 0,
			"swap": 0,
			"original_name": "m1.small",
			"extra_specs": {
			  "aggregate_instance_extra_specs:general": "true",
			  "hw:mem_page_size": "large",
			  "hw:vif_multiqueue_enabled": "true"
			}
		},
		"id": "9e5476bd-a4ec-4653-93d6-72c93aa682bb",
		"security_groups": [
			{
				"name": "default"
			}
		],
		"OS-SRV-USG:terminated_at": null,
		"OS-EXT-AZ:availability_zone": "nova",
		"user_id": "9349aff8be7545ac9d2f1d00999a23cd",
		"name": "merp",
		"created": "2014-09-25T13:04:41Z",
		"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
		"OS-DCF:diskConfig": "MANUAL",
		"os-extended-volumes:volumes_attached": [],
		"accessIPv4": "",
		"accessIPv6": "",
		"progress": 0,
		"OS-EXT-STS:power_state": 1,
		"config_drive": "",
		"metadata": {
			"env": "prod"
		}
	}
	]
}
`

// HandleServerListSuccessfully mocks server detail call.
func (m *SDMock) HandleServerListSuccessfully() {
	m.Mux.HandleFunc("/servers/detail", func(w http.ResponseWriter, r *http.Request) {
		testMethod(m.t, r, http.MethodGet)
		testHeader(m.t, r, "X-Auth-Token", tokenID)

		w.Header().Add("Content-Type", "application/json")
		fmt.Fprint(w, serverListBody)
	})
}

const listOutput = `
{
	"floatingips": [
		{
			"id": "03a77860-ae03-46c4-b502-caea11467a79",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"floating_ip_address": "10.10.10.1",
			"floating_network_id": "d02c4f18-d606-4864-b12a-1c9b39a46be2",
			"router_id": "f03af93b-4e8f-4f55-adcf-a0317782ede2",
			"port_id": "d5597901-48c8-4a69-a041-cfc5be158a04",
			"fixed_ip_address": null,
			"status": "ACTIVE",
			"description": "",
			"port_details": null,
			"dns_domain": "",
			"dns_name": "",
			"port_forwardings": [],
			"tags": [],
			"created_at": "2023-08-30T16:30:27Z",
			"updated_at": "2023-08-30T16:30:28Z"
		},
		{
			"id": "03e28c79-5a4c-491e-a4fe-3ff6bba830c6",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"floating_ip_address": "10.10.10.2",
			"floating_network_id": "d02c4f18-d606-4864-b12a-1c9b39a46be2",
			"router_id": "f03af93b-4e8f-4f55-adcf-a0317782ede2",
			"port_id": "4a45b012-0478-484d-8cf3-c8abdb194d08",
			"fixed_ip_address": "10.0.0.32",
			"status": "ACTIVE",
			"description": "",
			"port_details": {
			  "name": "ovn-lb-vip-c8f1d76f-1560-4445-8412-b817b0f0b922",
			  "network_id": "03200a39-b399-44f3-a778-6dbb93343a31",
			  "mac_address": "fa:16:3e:23:12:a3",
			  "admin_state_up": true,
			  "status": "ACTIVE",
			  "device_id": "",
			  "device_owner": ""
			},
			"dns_domain": "",
			"dns_name": "",
			"port_forwardings": [],
			"tags": [],
			"created_at": "2023-09-06T15:45:36Z",
			"updated_at": "2023-09-06T15:45:36Z"
		},
		{
			"id": "087fcdd2-1d13-4f72-9c0e-c759e796d558",
			"tenant_id": "fcad67a6189847c4aecfa3c81a05783b",
			"floating_ip_address": "10.10.10.4",
			"floating_network_id": "d02c4f18-d606-4864-b12a-1c9b39a46be2",
			"router_id": "f03af93b-4e8f-4f55-adcf-a0317782ede2",
			"port_id": "a0e244e8-7910-4427-b8d1-20470cad4f8a",
			"fixed_ip_address": "10.0.0.34",
			"status": "ACTIVE",
			"description": "",
			"port_details": {
			  "name": "ovn-lb-vip-5c0791f0-3878-41fb-a1c7-08c278828fe6",
			  "network_id": "03200a39-b399-44f3-a778-6dbb93343a31",
			  "mac_address": "fa:16:3e:5f:43:10",
			  "admin_state_up": true,
			  "status": "ACTIVE",
			  "device_id": "",
			  "device_owner": ""
			},
			"dns_domain": "",
			"dns_name": "",
			"port_forwardings": [],
			"tags": [],
			"created_at": "2024-01-24T13:30:50Z",
			"updated_at": "2024-01-24T13:30:51Z"
		}
	]
}
`

// HandleFloatingIPListSuccessfully mocks floating ips call.
func (m *SDMock) HandleFloatingIPListSuccessfully() {
	m.Mux.HandleFunc("/v2.0/floatingips", func(w http.ResponseWriter, r *http.Request) {
		testMethod(m.t, r, http.MethodGet)
		testHeader(m.t, r, "X-Auth-Token", tokenID)

		w.Header().Add("Content-Type", "application/json")
		fmt.Fprint(w, listOutput)
	})
}
