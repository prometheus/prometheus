package testing

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/baremetalintrospection/v1/introspection"
)

func TestLLDPTLVErrors(t *testing.T) {
	badInputs := []string{
		"[1]",
		"[1, 2]",
		"[\"foo\", \"bar\"]",
	}

	for _, input := range badInputs {
		var output introspection.LLDPTLVType
		err := json.Unmarshal([]byte(input), &output)
		if err == nil {
			t.Errorf("No JSON parse error for invalid LLDP TLV %s", input)
		}
		if !strings.Contains(err.Error(), "LLDP TLV") {
			t.Errorf("Unexpected JSON parse error \"%s\" for invalid LLDP TLV %s",
				err, input)
		}
	}
}

func TestExtraHardware(t *testing.T) {
	extraJson := `{
		"cpu": {
			"logical": {"number": 16},
			"physical": {
				"clock": 2105032704,
				"cores": 8,
				"flags": "lm fpu fpu_exception wp vme de"}
		},
		"disk": {
			"sda": {
				"rotational": 1,
				"vendor": "TEST"
			}
		},
		"firmware": {
			"bios": {
				"date": "01/01/1970",
				"vendor": "test"
			}
		},
		"ipmi": {
			"Fan1A RPM": {"unit": "RPM", "value": 3120},
			"Fan1B RPM": {"unit": "RPM", "value": 2280}
		},
		"memory": {
			"bank0": {
				"clock": 1600000000.0,
				"description": "DIMM DDR3 Synchronous Registered (Buffered) 1600 MHz (0.6 ns)"
			},
			"bank1": {
				"clock": 1600000000.0,
				"description": "DIMM DDR3 Synchronous Registered (Buffered) 1600 MHz (0.6 ns)"
			}
		},
		"network": {
			"em1": {
				"Autonegotiate": "on",
				"loopback": "off [fixed]"
			},
			"p2p1": {
				"Autonegotiate": "on",
				"loopback": "off [fixed]"
			}
		},
		"system": {
			"ipmi": {"channel": 1},
			"kernel": {"arch": "x86_64", "version": "3.10.0"},
			"motherboard": {"vendor": "Test"},
			"product": {"name": "test", "vendor": "Test"}
		}
	}`

	var output introspection.ExtraHardwareDataType
	err := json.Unmarshal([]byte(extraJson), &output)
	if err != nil {
		t.Errorf("Failed to unmarshal ExtraHardware data: %s", err)
	}
}

func TestHostnameInInventory(t *testing.T) {
	inventoryJson := `{
		"bmc_address":"192.167.2.134",
		"interfaces":[
		   {
			  "lldp":[],
			  "product":"0x0001",
			  "vendor":"0x1af4",
			  "name":"eth1",
			  "has_carrier":true,
			  "ipv4_address":"172.24.42.101",
			  "client_id":null,
			  "mac_address":"52:54:00:47:20:4d"
		   },
		   {
			  "lldp": [
				[1, "04112233aabbcc"],
				[5, "737730312d646973742d31622d623132"]
			  ],
			  "product":"0x0001",
			  "vendor":"0x1af4",
			  "name":"eth0",
			  "has_carrier":true,
			  "ipv4_address":"172.24.42.100",
			  "client_id":null,
			  "mac_address":"52:54:00:4e:3d:30"
		   }
		],
		"disks":[
		   {
			  "rotational":true,
			  "vendor":"0x1af4",
			  "name":"/dev/vda",
			  "hctl":null,
			  "wwn_vendor_extension":null,
			  "wwn_with_extension":null,
			  "model":"",
			  "wwn":null,
			  "serial":null,
			  "size":13958643712
		   }
		],
		"boot":{
		   "current_boot_mode":"bios",
		   "pxe_interface":"52:54:00:4e:3d:30"
		},
		"system_vendor":{
		   "serial_number":"Not Specified",
		   "product_name":"Bochs",
		   "manufacturer":"Bochs"
		},
		"memory":{
		   "physical_mb":2048,
		   "total":2105864192
		},
		"cpu":{
		   "count":2,
		   "frequency":"2100.084",
			"flags": [
				"fpu",
				"mmx",
				"fxsr",
			  	"sse",
			  	"sse2"
			],
			"architecture":"x86_64"
		},
		"hostname": "master-0"
	}`

	var output introspection.InventoryType
	err := json.Unmarshal([]byte(inventoryJson), &output)
	if err != nil {
		t.Errorf("Failed to unmarshal Inventory data: %s", err)
	}
}
