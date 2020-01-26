package ruletypes

import (
	"testing"

	"github.com/gophercloud/gophercloud/acceptance/clients"
	"github.com/gophercloud/gophercloud/acceptance/tools"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/qos/ruletypes"
)

func TestRuleTypes(t *testing.T) {
	client, err := clients.NewNetworkV2Client()
	if err != nil {
		t.Fatalf("Unable to create a network client: %v", err)
		return
	}

	page, err := ruletypes.ListRuleTypes(client).AllPages()
	if err != nil {
		t.Fatalf("Failed to list rule types pages: %v", err)
		return
	}

	ruleTypes, err := ruletypes.ExtractRuleTypes(page)
	if err != nil {
		t.Fatalf("Failed to list rule types: %v", err)
		return
	}

	tools.PrintResource(t, ruleTypes)

	if len(ruleTypes) > 0 {
		t.Logf("Trying to get rule type: %s", ruleTypes[0].Type)

		ruleType, err := ruletypes.GetRuleType(client, ruleTypes[0].Type).Extract()
		if err != nil {
			t.Fatalf("Failed to get rule type %s: %s", ruleTypes[0].Type, err)
		}

		tools.PrintResource(t, ruleType)
	}
}
