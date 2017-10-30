package eureka

import "strings"

func (c *Client) UnregisterInstance(appId, instanceId string) error {
	values := []string{"apps", appId, instanceId}
	path := strings.Join(values, "/")
	_, err := c.Delete(path)
	return err
}
