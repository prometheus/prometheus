package testing

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	fake "github.com/gophercloud/gophercloud/openstack/networking/v2/common"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/qos/policies"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/pagination"
	th "github.com/gophercloud/gophercloud/testhelper"
)

func TestGetPort(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/ports/65c0ee9f-d634-4522-8954-51021b570b0d", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, err := fmt.Fprintf(w, GetPortResponse)
		th.AssertNoErr(t, err)
	})

	var p struct {
		ports.Port
		policies.QoSPolicyExt
	}
	err := ports.Get(fake.ServiceClient(), "65c0ee9f-d634-4522-8954-51021b570b0d").ExtractInto(&p)
	th.AssertNoErr(t, err)

	th.AssertEquals(t, p.ID, "65c0ee9f-d634-4522-8954-51021b570b0d")
	th.AssertEquals(t, p.QoSPolicyID, "591e0597-39a6-4665-8149-2111d8de9a08")
}

func TestCreatePort(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/ports", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "POST")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		th.TestHeader(t, r, "Content-Type", "application/json")
		th.TestHeader(t, r, "Accept", "application/json")
		th.TestJSONRequest(t, r, CreatePortRequest)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		_, err := fmt.Fprintf(w, CreatePortResponse)
		th.AssertNoErr(t, err)
	})

	var p struct {
		ports.Port
		policies.QoSPolicyExt
	}
	portCreateOpts := ports.CreateOpts{
		NetworkID: "a87cc70a-3e15-4acf-8205-9b711a3531b7",
	}
	createOpts := policies.PortCreateOptsExt{
		CreateOptsBuilder: portCreateOpts,
		QoSPolicyID:       "591e0597-39a6-4665-8149-2111d8de9a08",
	}
	err := ports.Create(fake.ServiceClient(), createOpts).ExtractInto(&p)
	th.AssertNoErr(t, err)

	th.AssertEquals(t, p.NetworkID, "a87cc70a-3e15-4acf-8205-9b711a3531b7")
	th.AssertEquals(t, p.TenantID, "d6700c0c9ffa4f1cb322cd4a1f3906fa")
	th.AssertEquals(t, p.ID, "65c0ee9f-d634-4522-8954-51021b570b0d")
	th.AssertEquals(t, p.QoSPolicyID, "591e0597-39a6-4665-8149-2111d8de9a08")
}

func TestUpdatePortWithPolicy(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/ports/65c0ee9f-d634-4522-8954-51021b570b0d", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "PUT")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		th.TestHeader(t, r, "Content-Type", "application/json")
		th.TestHeader(t, r, "Accept", "application/json")
		th.TestJSONRequest(t, r, UpdatePortWithPolicyRequest)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, err := fmt.Fprintf(w, UpdatePortWithPolicyResponse)
		th.AssertNoErr(t, err)
	})

	policyID := "591e0597-39a6-4665-8149-2111d8de9a08"

	var p struct {
		ports.Port
		policies.QoSPolicyExt
	}
	portUpdateOpts := ports.UpdateOpts{}
	updateOpts := policies.PortUpdateOptsExt{
		UpdateOptsBuilder: portUpdateOpts,
		QoSPolicyID:       &policyID,
	}
	err := ports.Update(fake.ServiceClient(), "65c0ee9f-d634-4522-8954-51021b570b0d", updateOpts).ExtractInto(&p)
	th.AssertNoErr(t, err)

	th.AssertEquals(t, p.NetworkID, "a87cc70a-3e15-4acf-8205-9b711a3531b7")
	th.AssertEquals(t, p.TenantID, "d6700c0c9ffa4f1cb322cd4a1f3906fa")
	th.AssertEquals(t, p.ID, "65c0ee9f-d634-4522-8954-51021b570b0d")
	th.AssertEquals(t, p.QoSPolicyID, "591e0597-39a6-4665-8149-2111d8de9a08")
}

func TestUpdatePortWithoutPolicy(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/ports/65c0ee9f-d634-4522-8954-51021b570b0d", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "PUT")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		th.TestHeader(t, r, "Content-Type", "application/json")
		th.TestHeader(t, r, "Accept", "application/json")
		th.TestJSONRequest(t, r, UpdatePortWithoutPolicyRequest)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, err := fmt.Fprintf(w, UpdatePortWithoutPolicyResponse)
		th.AssertNoErr(t, err)
	})

	policyID := ""

	var p struct {
		ports.Port
		policies.QoSPolicyExt
	}
	portUpdateOpts := ports.UpdateOpts{}
	updateOpts := policies.PortUpdateOptsExt{
		UpdateOptsBuilder: portUpdateOpts,
		QoSPolicyID:       &policyID,
	}
	err := ports.Update(fake.ServiceClient(), "65c0ee9f-d634-4522-8954-51021b570b0d", updateOpts).ExtractInto(&p)
	th.AssertNoErr(t, err)

	th.AssertEquals(t, p.NetworkID, "a87cc70a-3e15-4acf-8205-9b711a3531b7")
	th.AssertEquals(t, p.TenantID, "d6700c0c9ffa4f1cb322cd4a1f3906fa")
	th.AssertEquals(t, p.ID, "65c0ee9f-d634-4522-8954-51021b570b0d")
	th.AssertEquals(t, p.QoSPolicyID, "")
}

func TestGetNetwork(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/networks/65c0ee9f-d634-4522-8954-51021b570b0d", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, err := fmt.Fprintf(w, GetNetworkResponse)
		th.AssertNoErr(t, err)
	})

	var n struct {
		networks.Network
		policies.QoSPolicyExt
	}
	err := networks.Get(fake.ServiceClient(), "65c0ee9f-d634-4522-8954-51021b570b0d").ExtractInto(&n)
	th.AssertNoErr(t, err)

	th.AssertEquals(t, n.ID, "65c0ee9f-d634-4522-8954-51021b570b0d")
	th.AssertEquals(t, n.QoSPolicyID, "591e0597-39a6-4665-8149-2111d8de9a08")
}

func TestCreateNetwork(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/networks", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "POST")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		th.TestHeader(t, r, "Content-Type", "application/json")
		th.TestHeader(t, r, "Accept", "application/json")
		th.TestJSONRequest(t, r, CreateNetworkRequest)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		_, err := fmt.Fprintf(w, CreateNetworkResponse)
		th.AssertNoErr(t, err)
	})

	var n struct {
		networks.Network
		policies.QoSPolicyExt
	}
	networkCreateOpts := networks.CreateOpts{
		Name: "private",
	}
	createOpts := policies.NetworkCreateOptsExt{
		CreateOptsBuilder: networkCreateOpts,
		QoSPolicyID:       "591e0597-39a6-4665-8149-2111d8de9a08",
	}
	err := networks.Create(fake.ServiceClient(), createOpts).ExtractInto(&n)
	th.AssertNoErr(t, err)

	th.AssertEquals(t, n.TenantID, "4fd44f30292945e481c7b8a0c8908869")
	th.AssertEquals(t, n.ID, "65c0ee9f-d634-4522-8954-51021b570b0d")
	th.AssertEquals(t, n.QoSPolicyID, "591e0597-39a6-4665-8149-2111d8de9a08")
}

func TestUpdateNetworkWithPolicy(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/networks/65c0ee9f-d634-4522-8954-51021b570b0d", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "PUT")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		th.TestHeader(t, r, "Content-Type", "application/json")
		th.TestHeader(t, r, "Accept", "application/json")
		th.TestJSONRequest(t, r, UpdateNetworkWithPolicyRequest)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, err := fmt.Fprintf(w, UpdateNetworkWithPolicyResponse)
		th.AssertNoErr(t, err)
	})

	policyID := "591e0597-39a6-4665-8149-2111d8de9a08"
	name := "updated"

	var n struct {
		networks.Network
		policies.QoSPolicyExt
	}
	networkUpdateOpts := networks.UpdateOpts{
		Name: &name,
	}
	updateOpts := policies.NetworkUpdateOptsExt{
		UpdateOptsBuilder: networkUpdateOpts,
		QoSPolicyID:       &policyID,
	}
	err := networks.Update(fake.ServiceClient(), "65c0ee9f-d634-4522-8954-51021b570b0d", updateOpts).ExtractInto(&n)
	th.AssertNoErr(t, err)

	th.AssertEquals(t, n.TenantID, "4fd44f30292945e481c7b8a0c8908869")
	th.AssertEquals(t, n.ID, "65c0ee9f-d634-4522-8954-51021b570b0d")
	th.AssertEquals(t, n.Name, "updated")
	th.AssertEquals(t, n.QoSPolicyID, "591e0597-39a6-4665-8149-2111d8de9a08")
}

func TestUpdateNetworkWithoutPolicy(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/networks/65c0ee9f-d634-4522-8954-51021b570b0d", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "PUT")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		th.TestHeader(t, r, "Content-Type", "application/json")
		th.TestHeader(t, r, "Accept", "application/json")
		th.TestJSONRequest(t, r, UpdateNetworkWithoutPolicyRequest)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, err := fmt.Fprintf(w, UpdateNetworkWithoutPolicyResponse)
		th.AssertNoErr(t, err)
	})

	policyID := ""

	var n struct {
		networks.Network
		policies.QoSPolicyExt
	}
	networkUpdateOpts := networks.UpdateOpts{}
	updateOpts := policies.NetworkUpdateOptsExt{
		UpdateOptsBuilder: networkUpdateOpts,
		QoSPolicyID:       &policyID,
	}
	err := networks.Update(fake.ServiceClient(), "65c0ee9f-d634-4522-8954-51021b570b0d", updateOpts).ExtractInto(&n)
	th.AssertNoErr(t, err)

	th.AssertEquals(t, n.TenantID, "4fd44f30292945e481c7b8a0c8908869")
	th.AssertEquals(t, n.ID, "65c0ee9f-d634-4522-8954-51021b570b0d")
	th.AssertEquals(t, n.QoSPolicyID, "")
}

func TestListPolicies(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, ListPoliciesResponse)
	})

	count := 0

	err := policies.List(fake.ServiceClient(), policies.ListOpts{}).EachPage(func(page pagination.Page) (bool, error) {
		count++
		actual, err := policies.ExtractPolicies(page)
		if err != nil {
			t.Errorf("Failed to extract policies: %v", err)
			return false, nil
		}

		expected := []policies.Policy{
			Policy1,
			Policy2,
		}

		th.CheckDeepEquals(t, expected, actual)

		return true, nil
	})
	th.AssertNoErr(t, err)

	if count != 1 {
		t.Errorf("Expected 1 page, got %d", count)
	}
}

func TestGetPolicy(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/30a57f4a-336b-4382-8275-d708babd2241", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, GetPolicyResponse)
	})

	p, err := policies.Get(fake.ServiceClient(), "30a57f4a-336b-4382-8275-d708babd2241").Extract()
	th.AssertNoErr(t, err)

	th.AssertEquals(t, "bw-limiter", p.Name)
	th.AssertDeepEquals(t, []string{}, p.Tags)
	th.AssertDeepEquals(t, []map[string]interface{}{
		{
			"max_kbps":       float64(3000),
			"direction":      "egress",
			"qos_policy_id":  "d6ae28ce-fcb5-4180-aa62-d260a27e09ae",
			"type":           "bandwidth_limit",
			"id":             "30a57f4a-336b-4382-8275-d708babd2241",
			"max_burst_kbps": float64(300),
		},
	}, p.Rules)
	th.AssertEquals(t, "a77cbe0998374aed9a6798ad6c61677e", p.TenantID)
	th.AssertEquals(t, "a77cbe0998374aed9a6798ad6c61677e", p.ProjectID)
	th.AssertEquals(t, time.Date(2019, 5, 19, 11, 17, 50, 0, time.UTC), p.CreatedAt)
	th.AssertEquals(t, time.Date(2019, 5, 19, 11, 17, 57, 0, time.UTC), p.UpdatedAt)
	th.AssertEquals(t, 1, p.RevisionNumber)
	th.AssertEquals(t, "d6ae28ce-fcb5-4180-aa62-d260a27e09ae", p.ID)
}

func TestCreatePolicy(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "POST")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		th.TestHeader(t, r, "Content-Type", "application/json")
		th.TestHeader(t, r, "Accept", "application/json")
		th.TestJSONRequest(t, r, CreatePolicyRequest)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		fmt.Fprintf(w, CreatePolicyResponse)
	})

	opts := policies.CreateOpts{
		Name:        "shared-default-policy",
		Shared:      true,
		IsDefault:   true,
		Description: "use-me",
	}
	p, err := policies.Create(fake.ServiceClient(), opts).Extract()
	th.AssertNoErr(t, err)

	th.AssertEquals(t, "shared-default-policy", p.Name)
	th.AssertEquals(t, true, p.Shared)
	th.AssertEquals(t, true, p.IsDefault)
	th.AssertEquals(t, "use-me", p.Description)
	th.AssertEquals(t, "a77cbe0998374aed9a6798ad6c61677e", p.TenantID)
	th.AssertEquals(t, "a77cbe0998374aed9a6798ad6c61677e", p.ProjectID)
	th.AssertEquals(t, time.Date(2019, 5, 19, 11, 17, 50, 0, time.UTC), p.CreatedAt)
	th.AssertEquals(t, time.Date(2019, 5, 19, 11, 17, 57, 0, time.UTC), p.UpdatedAt)
	th.AssertEquals(t, 0, p.RevisionNumber)
	th.AssertEquals(t, "d6ae28ce-fcb5-4180-aa62-d260a27e09ae", p.ID)
}

func TestUpdatePolicy(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/d6ae28ce-fcb5-4180-aa62-d260a27e09ae", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "PUT")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		th.TestHeader(t, r, "Content-Type", "application/json")
		th.TestHeader(t, r, "Accept", "application/json")
		th.TestJSONRequest(t, r, UpdatePolicyRequest)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, UpdatePolicyResponse)
	})

	shared := true
	description := ""
	opts := policies.UpdateOpts{
		Name:        "new-name",
		Shared:      &shared,
		Description: &description,
	}
	p, err := policies.Update(fake.ServiceClient(), "d6ae28ce-fcb5-4180-aa62-d260a27e09ae", opts).Extract()
	th.AssertNoErr(t, err)

	th.AssertEquals(t, "new-name", p.Name)
	th.AssertEquals(t, true, p.Shared)
	th.AssertEquals(t, false, p.IsDefault)
	th.AssertEquals(t, "", p.Description)
	th.AssertEquals(t, "a77cbe0998374aed9a6798ad6c61677e", p.TenantID)
	th.AssertEquals(t, "a77cbe0998374aed9a6798ad6c61677e", p.ProjectID)
	th.AssertEquals(t, time.Date(2019, 5, 19, 11, 17, 50, 0, time.UTC), p.CreatedAt)
	th.AssertEquals(t, time.Date(2019, 6, 1, 13, 17, 57, 0, time.UTC), p.UpdatedAt)
	th.AssertEquals(t, 1, p.RevisionNumber)
	th.AssertEquals(t, "d6ae28ce-fcb5-4180-aa62-d260a27e09ae", p.ID)
}

func TestDeletePolicy(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	th.Mux.HandleFunc("/v2.0/qos/policies/d6ae28ce-fcb5-4180-aa62-d260a27e09ae", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "DELETE")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		w.WriteHeader(http.StatusNoContent)
	})

	res := policies.Delete(fake.ServiceClient(), "d6ae28ce-fcb5-4180-aa62-d260a27e09ae")
	th.AssertNoErr(t, res.Err)
}
