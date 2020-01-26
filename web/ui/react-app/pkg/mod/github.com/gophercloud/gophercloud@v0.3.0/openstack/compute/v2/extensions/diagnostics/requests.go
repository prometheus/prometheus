package diagnostics

import (
	"github.com/gophercloud/gophercloud"
)

// Diagnostics
func Get(client *gophercloud.ServiceClient, serverId string) (r serverDiagnosticsResult) {
	_, r.Err = client.Get(serverDiagnosticsURL(client, serverId), &r.Body, nil)
	return
}
