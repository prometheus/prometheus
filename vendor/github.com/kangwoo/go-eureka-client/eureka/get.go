package eureka

import (
	"encoding/xml"
	"strings"
)

func (c *Client) GetApplications() (*Applications, error) {
	response, err := c.Get("apps")
	if err != nil {
		return nil, err
	}
	var applications *Applications = new(Applications)
	err = xml.Unmarshal(response.Body, applications)
	return applications, err
}

func (c *Client) GetApplication(appId string) (*Application, error) {
	values := []string{"apps", appId}
	path := strings.Join(values, "/")
	response, err := c.Get(path)
	if err != nil {
		return nil, err
	}
	var application *Application = new(Application)
	err = xml.Unmarshal(response.Body, application)
	return application, err
}

func (c *Client) GetInstance(appId, instanceId string) (*InstanceInfo, error) {
	values := []string{"apps", appId, instanceId}
	path := strings.Join(values, "/")
	response, err := c.Get(path)
	if err != nil {
		return nil, err
	}
	var instance *InstanceInfo = new(InstanceInfo)
	err = xml.Unmarshal(response.Body, instance)
	return instance, err
}

func (c *Client) GetVIP(vipId string) (*Applications, error) {
	values := []string{"vips", vipId}
	path := strings.Join(values, "/")
	response, err := c.Get(path)
	if err != nil {
		return nil, err
	}
	var applications *Applications = new(Applications)
	err = xml.Unmarshal(response.Body, applications)
	return applications, err
}

func (c *Client) GetSVIP(svipId string) (*Applications, error) {
	values := []string{"svips", svipId}
	path := strings.Join(values, "/")
	response, err := c.Get(path)
	if err != nil {
		return nil, err
	}
	var applications *Applications = new(Applications)
	err = xml.Unmarshal(response.Body, applications)
	return applications, err
}
