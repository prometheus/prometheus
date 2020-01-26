package apiversions

import (
	"github.com/gophercloud/gophercloud"
)

// List lists all the API versions available to end users.
func List(client *gophercloud.ServiceClient) (r ListResult) {
	_, r.Err = client.Get(listURL(client), &r.Body, nil)
	return
}

// Get will get a specific API version, specified by major ID.
func Get(client *gophercloud.ServiceClient, v string) (r GetResult) {
	_, r.Err = client.Get(getURL(client, v), &r.Body, nil)
	return
}
