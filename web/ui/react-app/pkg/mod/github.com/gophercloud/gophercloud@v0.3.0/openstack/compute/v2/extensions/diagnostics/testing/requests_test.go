package testing

import (
	"testing"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/diagnostics"
	th "github.com/gophercloud/gophercloud/testhelper"
	"github.com/gophercloud/gophercloud/testhelper/client"
)

func TestGetDiagnostics(t *testing.T) {
	th.SetupHTTP()
	defer th.TeardownHTTP()

	HandleDiagnosticGetSuccessfully(t)

	expected := map[string]interface{}{"cpu0_time": float64(173), "memory": float64(524288)}

	res, err := diagnostics.Get(client.ServiceClient(), "1234asdf").Extract()
	th.AssertNoErr(t, err)

	th.AssertDeepEquals(t, expected, res)
}
