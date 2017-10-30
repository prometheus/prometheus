package eureka

import "strings"

func (c *Client) SendHeartbeat(appId, instanceId string) error {
	values := []string{"apps", appId, instanceId}
	path := strings.Join(values, "/")
	_, err := c.Put(path, nil)
	return err
}
