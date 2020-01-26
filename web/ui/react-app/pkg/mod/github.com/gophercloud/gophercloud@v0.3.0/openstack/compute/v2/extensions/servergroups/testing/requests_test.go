package testing

import (
	"testing"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/servergroups"
	"github.com/gophercloud/gophercloud/pagination"
	th "github.com/gophercloud/gophercloud/testhelper"
	"github.com/gophercloud/gophercloud/testhelper/client"
)

func TestList(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()
	HandleListSuccessfully(t)

	count := 0
	err := servergroups.List(client.ServiceClient()).EachPage(func(page pagination.Page) (bool, error) {
		count++
		actual, err := servergroups.ExtractServerGroups(page)
		th.AssertNoErr(t, err)
		th.CheckDeepEquals(t, ExpectedServerGroupSlice, actual)

		return true, nil
	})
	th.AssertNoErr(t, err)
	th.CheckEquals(t, 1, count)
}

func TestCreate(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()
	HandleCreateSuccessfully(t)

	actual, err := servergroups.Create(client.ServiceClient(), servergroups.CreateOpts{
		Name:     "test",
		Policies: []string{"anti-affinity"},
	}).Extract()
	th.AssertNoErr(t, err)
	th.CheckDeepEquals(t, &CreatedServerGroup, actual)
}

func TestCreateMicroversion(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()
	HandleCreateMicroversionSuccessfully(t)

	result := servergroups.Create(client.ServiceClient(), servergroups.CreateOpts{
		Name:     "test",
		Policies: []string{"anti-affinity"},
		Policy:   "anti-affinity",
		Rules: &servergroups.Rules{
			MaxServerPerHost: 3,
		},
	})

	// Extract basic fields.
	actual, err := result.Extract()
	th.AssertNoErr(t, err)
	th.CheckDeepEquals(t, &CreatedServerGroup, actual)

	// Extract additional fields.
	policy, err := servergroups.ExtractPolicy(result.Result)
	th.AssertNoErr(t, err)
	th.AssertEquals(t, "anti-affinity", policy)

	rules, err := servergroups.ExtractRules(result.Result)
	th.AssertNoErr(t, err)
	th.AssertEquals(t, 3, rules.MaxServerPerHost)
}

func TestGet(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()
	HandleGetSuccessfully(t)

	actual, err := servergroups.Get(client.ServiceClient(), "4d8c3732-a248-40ed-bebc-539a6ffd25c0").Extract()
	th.AssertNoErr(t, err)
	th.CheckDeepEquals(t, &FirstServerGroup, actual)
}

func TestGetMicroversion(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()
	HandleGetMicroversionSuccessfully(t)

	result := servergroups.Get(client.ServiceClient(), "4d8c3732-a248-40ed-bebc-539a6ffd25c0")

	// Extract basic fields.
	actual, err := result.Extract()
	th.AssertNoErr(t, err)
	th.CheckDeepEquals(t, &FirstServerGroup, actual)

	// Extract additional fields.
	policy, err := servergroups.ExtractPolicy(result.Result)
	th.AssertNoErr(t, err)
	th.AssertEquals(t, "anti-affinity", policy)

	rules, err := servergroups.ExtractRules(result.Result)
	th.AssertNoErr(t, err)
	th.AssertEquals(t, 3, rules.MaxServerPerHost)
}

func TestDelete(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()
	HandleDeleteSuccessfully(t)

	err := servergroups.Delete(client.ServiceClient(), "616fb98f-46ca-475e-917e-2563e5a8cd19").ExtractErr()
	th.AssertNoErr(t, err)
}
