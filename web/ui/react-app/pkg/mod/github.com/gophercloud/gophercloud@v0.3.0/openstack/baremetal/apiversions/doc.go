/*
Package apiversions provides information about the versions supported by a specific Ironic API.

	Example to list versions

		allVersions, err := apiversions.List(client.ServiceClient()).AllPages()

	Example to get a specific version

		actual, err := apiversions.Get(client.ServiceClient(), "v1").Extract()

*/
package apiversions
