package oauth1

import "github.com/gophercloud/gophercloud"

func consumersURL(c *gophercloud.ServiceClient) string {
	return c.ServiceURL("OS-OAUTH1", "consumers")
}

func consumerURL(c *gophercloud.ServiceClient, id string) string {
	return c.ServiceURL("OS-OAUTH1", "consumers", id)
}

func requestTokenURL(c *gophercloud.ServiceClient) string {
	return c.ServiceURL("OS-OAUTH1", "request_token")
}

func authorizeTokenURL(c *gophercloud.ServiceClient, id string) string {
	return c.ServiceURL("OS-OAUTH1", "authorize", id)
}

func createAccessTokenURL(c *gophercloud.ServiceClient) string {
	return c.ServiceURL("OS-OAUTH1", "access_token")
}

func userAccessTokensURL(c *gophercloud.ServiceClient, userID string) string {
	return c.ServiceURL("users", userID, "OS-OAUTH1", "access_tokens")
}

func userAccessTokenURL(c *gophercloud.ServiceClient, userID string, id string) string {
	return c.ServiceURL("users", userID, "OS-OAUTH1", "access_tokens", id)
}

func userAccessTokenRolesURL(c *gophercloud.ServiceClient, userID string, id string) string {
	return c.ServiceURL("users", userID, "OS-OAUTH1", "access_tokens", id, "roles")
}

func userAccessTokenRoleURL(c *gophercloud.ServiceClient, userID string, id string, roleID string) string {
	return c.ServiceURL("users", userID, "OS-OAUTH1", "access_tokens", id, "roles", roleID)
}

func authURL(c *gophercloud.ServiceClient) string {
	return c.ServiceURL("auth", "tokens")
}
