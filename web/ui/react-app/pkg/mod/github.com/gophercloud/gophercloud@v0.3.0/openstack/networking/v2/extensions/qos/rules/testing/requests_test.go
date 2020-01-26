package testing

import (
	"fmt"
	"net/http"
	"testing"

	fake "github.com/gophercloud/gophercloud/openstack/networking/v2/common"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/qos/rules"
	"github.com/gophercloud/gophercloud/pagination"
	th "github.com/gophercloud/gophercloud/testhelper"
)

func TestListBandwidthLimitRule(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/501005fa-3b56-4061-aaca-3f24995112e1/bandwidth_limit_rules", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, BandwidthLimitRulesListResult)
	})

	count := 0

	err := rules.ListBandwidthLimitRules(
		fake.ServiceClient(),
		"501005fa-3b56-4061-aaca-3f24995112e1",
		rules.BandwidthLimitRulesListOpts{},
	).EachPage(func(page pagination.Page) (bool, error) {
		count++
		actual, err := rules.ExtractBandwidthLimitRules(page)
		if err != nil {
			t.Errorf("Failed to extract bandwith limit rules: %v", err)
			return false, nil
		}

		expected := []rules.BandwidthLimitRule{
			{
				ID:           "30a57f4a-336b-4382-8275-d708babd2241",
				MaxKBps:      3000,
				MaxBurstKBps: 300,
				Direction:    "egress",
			},
		}

		th.CheckDeepEquals(t, expected, actual)

		return true, nil
	})
	th.AssertNoErr(t, err)

	if count != 1 {
		t.Errorf("Expected 1 page, got %d", count)
	}
}

func TestGetBandwidthLimitRule(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/501005fa-3b56-4061-aaca-3f24995112e1/bandwidth_limit_rules/30a57f4a-336b-4382-8275-d708babd2241", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, BandwidthLimitRulesGetResult)
	})

	r, err := rules.GetBandwidthLimitRule(fake.ServiceClient(), "501005fa-3b56-4061-aaca-3f24995112e1", "30a57f4a-336b-4382-8275-d708babd2241").ExtractBandwidthLimitRule()
	th.AssertNoErr(t, err)

	th.AssertEquals(t, r.ID, "30a57f4a-336b-4382-8275-d708babd2241")
	th.AssertEquals(t, r.Direction, "egress")
	th.AssertEquals(t, r.MaxBurstKBps, 300)
	th.AssertEquals(t, r.MaxKBps, 3000)
}

func TestCreateBandwidthLimitRule(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/501005fa-3b56-4061-aaca-3f24995112e1/bandwidth_limit_rules", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "POST")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		th.TestHeader(t, r, "Content-Type", "application/json")
		th.TestHeader(t, r, "Accept", "application/json")
		th.TestJSONRequest(t, r, BandwidthLimitRulesCreateRequest)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		fmt.Fprintf(w, BandwidthLimitRulesCreateResult)
	})

	opts := rules.CreateBandwidthLimitRuleOpts{
		MaxKBps:      2000,
		MaxBurstKBps: 200,
	}
	r, err := rules.CreateBandwidthLimitRule(fake.ServiceClient(), "501005fa-3b56-4061-aaca-3f24995112e1", opts).ExtractBandwidthLimitRule()
	th.AssertNoErr(t, err)

	th.AssertEquals(t, 200, r.MaxBurstKBps)
	th.AssertEquals(t, 2000, r.MaxKBps)
}

func TestUpdateBandwidthLimitRule(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/501005fa-3b56-4061-aaca-3f24995112e1/bandwidth_limit_rules/30a57f4a-336b-4382-8275-d708babd2241", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "PUT")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		th.TestHeader(t, r, "Content-Type", "application/json")
		th.TestHeader(t, r, "Accept", "application/json")
		th.TestJSONRequest(t, r, BandwidthLimitRulesUpdateRequest)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, BandwidthLimitRulesUpdateResult)
	})

	maxKBps := 500
	maxBurstKBps := 0
	opts := rules.UpdateBandwidthLimitRuleOpts{
		MaxKBps:      &maxKBps,
		MaxBurstKBps: &maxBurstKBps,
	}
	r, err := rules.UpdateBandwidthLimitRule(fake.ServiceClient(), "501005fa-3b56-4061-aaca-3f24995112e1", "30a57f4a-336b-4382-8275-d708babd2241", opts).ExtractBandwidthLimitRule()
	th.AssertNoErr(t, err)

	th.AssertEquals(t, 0, r.MaxBurstKBps)
	th.AssertEquals(t, 500, r.MaxKBps)
}

func TestDeleteBandwidthLimitRule(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/501005fa-3b56-4061-aaca-3f24995112e1/bandwidth_limit_rules/30a57f4a-336b-4382-8275-d708babd2241", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "DELETE")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		w.WriteHeader(http.StatusNoContent)
	})

	res := rules.DeleteBandwidthLimitRule(fake.ServiceClient(), "501005fa-3b56-4061-aaca-3f24995112e1", "30a57f4a-336b-4382-8275-d708babd2241")
	th.AssertNoErr(t, res.Err)
}

func TestListDSCPMarkingRule(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/501005fa-3b56-4061-aaca-3f24995112e1/dscp_marking_rules", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, DSCPMarkingRulesListResult)
	})

	count := 0

	err := rules.ListDSCPMarkingRules(
		fake.ServiceClient(),
		"501005fa-3b56-4061-aaca-3f24995112e1",
		rules.DSCPMarkingRulesListOpts{},
	).EachPage(func(page pagination.Page) (bool, error) {
		count++
		actual, err := rules.ExtractDSCPMarkingRules(page)
		if err != nil {
			t.Errorf("Failed to extract DSCP marking rules: %v", err)
			return false, nil
		}

		expected := []rules.DSCPMarkingRule{
			{
				ID:       "30a57f4a-336b-4382-8275-d708babd2241",
				DSCPMark: 20,
			},
		}

		th.CheckDeepEquals(t, expected, actual)

		return true, nil
	})
	th.AssertNoErr(t, err)

	if count != 1 {
		t.Errorf("Expected 1 page, got %d", count)
	}
}

func TestGetDSCPMarkingRule(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/501005fa-3b56-4061-aaca-3f24995112e1/dscp_marking_rules/30a57f4a-336b-4382-8275-d708babd2241", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, DSCPMarkingRuleGetResult)
	})

	r, err := rules.GetDSCPMarkingRule(fake.ServiceClient(), "501005fa-3b56-4061-aaca-3f24995112e1", "30a57f4a-336b-4382-8275-d708babd2241").ExtractDSCPMarkingRule()
	th.AssertNoErr(t, err)

	th.AssertEquals(t, r.ID, "30a57f4a-336b-4382-8275-d708babd2241")
	th.AssertEquals(t, 26, r.DSCPMark)
}

func TestCreateDSCPMarkingRule(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/501005fa-3b56-4061-aaca-3f24995112e1/dscp_marking_rules", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "POST")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		th.TestHeader(t, r, "Content-Type", "application/json")
		th.TestHeader(t, r, "Accept", "application/json")
		th.TestJSONRequest(t, r, DSCPMarkingRuleCreateRequest)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		fmt.Fprintf(w, DSCPMarkingRuleCreateResult)
	})

	opts := rules.CreateDSCPMarkingRuleOpts{
		DSCPMark: 20,
	}
	r, err := rules.CreateDSCPMarkingRule(fake.ServiceClient(), "501005fa-3b56-4061-aaca-3f24995112e1", opts).ExtractDSCPMarkingRule()
	th.AssertNoErr(t, err)

	th.AssertEquals(t, "30a57f4a-336b-4382-8275-d708babd2241", r.ID)
	th.AssertEquals(t, 20, r.DSCPMark)
}

func TestUpdateDSCPMarkingRule(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/501005fa-3b56-4061-aaca-3f24995112e1/dscp_marking_rules/30a57f4a-336b-4382-8275-d708babd2241", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "PUT")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		th.TestHeader(t, r, "Content-Type", "application/json")
		th.TestHeader(t, r, "Accept", "application/json")
		th.TestJSONRequest(t, r, DSCPMarkingRuleUpdateRequest)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, DSCPMarkingRuleUpdateResult)
	})

	dscpMark := 26
	opts := rules.UpdateDSCPMarkingRuleOpts{
		DSCPMark: &dscpMark,
	}
	r, err := rules.UpdateDSCPMarkingRule(fake.ServiceClient(), "501005fa-3b56-4061-aaca-3f24995112e1", "30a57f4a-336b-4382-8275-d708babd2241", opts).ExtractDSCPMarkingRule()
	th.AssertNoErr(t, err)

	th.AssertEquals(t, "30a57f4a-336b-4382-8275-d708babd2241", r.ID)
	th.AssertEquals(t, 26, r.DSCPMark)
}

func TestDeleteDSCPMarkingRule(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/501005fa-3b56-4061-aaca-3f24995112e1/dscp_marking_rules/30a57f4a-336b-4382-8275-d708babd2241", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "DELETE")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		w.WriteHeader(http.StatusNoContent)
	})

	res := rules.DeleteDSCPMarkingRule(fake.ServiceClient(), "501005fa-3b56-4061-aaca-3f24995112e1", "30a57f4a-336b-4382-8275-d708babd2241")
	th.AssertNoErr(t, res.Err)
}

func TestListMinimumBandwidthRule(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/501005fa-3b56-4061-aaca-3f24995112e1/minimum_bandwidth_rules", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, MinimumBandwidthRulesListResult)
	})

	count := 0

	err := rules.ListMinimumBandwidthRules(
		fake.ServiceClient(),
		"501005fa-3b56-4061-aaca-3f24995112e1",
		rules.MinimumBandwidthRulesListOpts{},
	).EachPage(func(page pagination.Page) (bool, error) {
		count++
		actual, err := rules.ExtractMinimumBandwidthRules(page)
		if err != nil {
			t.Errorf("Failed to extract minimum bandwith rules: %v", err)
			return false, nil
		}

		expected := []rules.MinimumBandwidthRule{
			{
				ID:        "30a57f4a-336b-4382-8275-d708babd2241",
				Direction: "egress",
				MinKBps:   3000,
			},
		}

		th.CheckDeepEquals(t, expected, actual)

		return true, nil
	})
	th.AssertNoErr(t, err)

	if count != 1 {
		t.Errorf("Expected 1 page, got %d", count)
	}
}

func TestGetMinimumBandwidthRule(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/501005fa-3b56-4061-aaca-3f24995112e1/minimum_bandwidth_rules/30a57f4a-336b-4382-8275-d708babd2241", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, MinimumBandwidthRulesGetResult)
	})

	r, err := rules.GetMinimumBandwidthRule(fake.ServiceClient(), "501005fa-3b56-4061-aaca-3f24995112e1", "30a57f4a-336b-4382-8275-d708babd2241").ExtractMinimumBandwidthRule()
	th.AssertNoErr(t, err)

	th.AssertEquals(t, r.ID, "30a57f4a-336b-4382-8275-d708babd2241")
	th.AssertEquals(t, r.Direction, "egress")
	th.AssertEquals(t, r.MinKBps, 3000)
}

func TestCreateMinimumBandwidthRule(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/501005fa-3b56-4061-aaca-3f24995112e1/minimum_bandwidth_rules", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "POST")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		th.TestHeader(t, r, "Content-Type", "application/json")
		th.TestHeader(t, r, "Accept", "application/json")
		th.TestJSONRequest(t, r, MinimumBandwidthRulesCreateRequest)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		fmt.Fprintf(w, MinimumBandwidthRulesCreateResult)
	})

	opts := rules.CreateMinimumBandwidthRuleOpts{
		MinKBps: 2000,
	}
	r, err := rules.CreateMinimumBandwidthRule(fake.ServiceClient(), "501005fa-3b56-4061-aaca-3f24995112e1", opts).ExtractMinimumBandwidthRule()
	th.AssertNoErr(t, err)

	th.AssertEquals(t, 2000, r.MinKBps)
}

func TestUpdateMinimumBandwidthRule(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/501005fa-3b56-4061-aaca-3f24995112e1/minimum_bandwidth_rules/30a57f4a-336b-4382-8275-d708babd2241", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "PUT")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		th.TestHeader(t, r, "Content-Type", "application/json")
		th.TestHeader(t, r, "Accept", "application/json")
		th.TestJSONRequest(t, r, MinimumBandwidthRulesUpdateRequest)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, MinimumBandwidthRulesUpdateResult)
	})

	minKBps := 500
	opts := rules.UpdateMinimumBandwidthRuleOpts{
		MinKBps: &minKBps,
	}
	r, err := rules.UpdateMinimumBandwidthRule(fake.ServiceClient(), "501005fa-3b56-4061-aaca-3f24995112e1", "30a57f4a-336b-4382-8275-d708babd2241", opts).ExtractMinimumBandwidthRule()
	th.AssertNoErr(t, err)

	th.AssertEquals(t, 500, r.MinKBps)
}

func TestDeleteMinimumBandwidthRule(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/501005fa-3b56-4061-aaca-3f24995112e1/minimum_bandwidth_rules/30a57f4a-336b-4382-8275-d708babd2241", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "DELETE")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		w.WriteHeader(http.StatusNoContent)
	})

	res := rules.DeleteMinimumBandwidthRule(fake.ServiceClient(), "501005fa-3b56-4061-aaca-3f24995112e1", "30a57f4a-336b-4382-8275-d708babd2241")
	th.AssertNoErr(t, res.Err)
}
