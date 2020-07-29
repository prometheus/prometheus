package hypervisors

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/pagination"
)

// List makes a request against the API to list hypervisors.
func List(client *gophercloud.ServiceClient) pagination.Pager {
	return pagination.NewPager(client, hypervisorsListDetailURL(client), func(r pagination.PageResult) pagination.Page {
		return HypervisorPage{pagination.SinglePageBase(r)}
	})
}

// Statistics makes a request against the API to get hypervisors statistics.
func GetStatistics(client *gophercloud.ServiceClient) (r StatisticsResult) {
	resp, err := client.Get(hypervisorsStatisticsURL(client), &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{200},
	})
	_, r.Header, r.Err = gophercloud.ParseResponse(resp, err)
	return
}

// Get makes a request against the API to get details for specific hypervisor.
func Get(client *gophercloud.ServiceClient, hypervisorID string) (r HypervisorResult) {
	resp, err := client.Get(hypervisorsGetURL(client, hypervisorID), &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{200},
	})
	_, r.Header, r.Err = gophercloud.ParseResponse(resp, err)
	return
}

// GetUptime makes a request against the API to get uptime for specific hypervisor.
func GetUptime(client *gophercloud.ServiceClient, hypervisorID string) (r UptimeResult) {
	resp, err := client.Get(hypervisorsUptimeURL(client, hypervisorID), &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{200},
	})
	_, r.Header, r.Err = gophercloud.ParseResponse(resp, err)
	return
}
