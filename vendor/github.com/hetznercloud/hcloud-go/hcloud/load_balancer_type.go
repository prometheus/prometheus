package hcloud

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/hetznercloud/hcloud-go/hcloud/schema"
)

// LoadBalancerType represents a LoadBalancer type in the Hetzner Cloud.
type LoadBalancerType struct {
	ID                      int
	Name                    string
	Description             string
	MaxConnections          int
	MaxServices             int
	MaxTargets              int
	MaxAssignedCertificates int
	Pricings                []LoadBalancerTypeLocationPricing
}

// LoadBalancerTypeClient is a client for the Load Balancer types API.
type LoadBalancerTypeClient struct {
	client *Client
}

// GetByID retrieves a Load Balancer type by its ID. If the Load Balancer type does not exist, nil is returned.
func (c *LoadBalancerTypeClient) GetByID(ctx context.Context, id int) (*LoadBalancerType, *Response, error) {
	req, err := c.client.NewRequest(ctx, "GET", fmt.Sprintf("/load_balancer_types/%d", id), nil)
	if err != nil {
		return nil, nil, err
	}

	var body schema.LoadBalancerTypeGetResponse
	resp, err := c.client.Do(req, &body)
	if err != nil {
		if IsError(err, ErrorCodeNotFound) {
			return nil, resp, nil
		}
		return nil, nil, err
	}
	return LoadBalancerTypeFromSchema(body.LoadBalancerType), resp, nil
}

// GetByName retrieves a Load Balancer type by its name. If the Load Balancer type does not exist, nil is returned.
func (c *LoadBalancerTypeClient) GetByName(ctx context.Context, name string) (*LoadBalancerType, *Response, error) {
	if name == "" {
		return nil, nil, nil
	}
	LoadBalancerTypes, response, err := c.List(ctx, LoadBalancerTypeListOpts{Name: name})
	if len(LoadBalancerTypes) == 0 {
		return nil, response, err
	}
	return LoadBalancerTypes[0], response, err
}

// Get retrieves a Load Balancer type by its ID if the input can be parsed as an integer, otherwise it
// retrieves a Load Balancer type by its name. If the Load Balancer type does not exist, nil is returned.
func (c *LoadBalancerTypeClient) Get(ctx context.Context, idOrName string) (*LoadBalancerType, *Response, error) {
	if id, err := strconv.Atoi(idOrName); err == nil {
		return c.GetByID(ctx, int(id))
	}
	return c.GetByName(ctx, idOrName)
}

// LoadBalancerTypeListOpts specifies options for listing Load Balancer types.
type LoadBalancerTypeListOpts struct {
	ListOpts
	Name string
}

func (l LoadBalancerTypeListOpts) values() url.Values {
	vals := l.ListOpts.values()
	if l.Name != "" {
		vals.Add("name", l.Name)
	}
	return vals
}

// List returns a list of Load Balancer types for a specific page.
//
// Please note that filters specified in opts are not taken into account
// when their value corresponds to their zero value or when they are empty.
func (c *LoadBalancerTypeClient) List(ctx context.Context, opts LoadBalancerTypeListOpts) ([]*LoadBalancerType, *Response, error) {
	path := "/load_balancer_types?" + opts.values().Encode()
	req, err := c.client.NewRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, nil, err
	}

	var body schema.LoadBalancerTypeListResponse
	resp, err := c.client.Do(req, &body)
	if err != nil {
		return nil, nil, err
	}
	LoadBalancerTypes := make([]*LoadBalancerType, 0, len(body.LoadBalancerTypes))
	for _, s := range body.LoadBalancerTypes {
		LoadBalancerTypes = append(LoadBalancerTypes, LoadBalancerTypeFromSchema(s))
	}
	return LoadBalancerTypes, resp, nil
}

// All returns all Load Balancer types.
func (c *LoadBalancerTypeClient) All(ctx context.Context) ([]*LoadBalancerType, error) {
	allLoadBalancerTypes := []*LoadBalancerType{}

	opts := LoadBalancerTypeListOpts{}
	opts.PerPage = 50

	_, err := c.client.all(func(page int) (*Response, error) {
		opts.Page = page
		LoadBalancerTypes, resp, err := c.List(ctx, opts)
		if err != nil {
			return resp, err
		}
		allLoadBalancerTypes = append(allLoadBalancerTypes, LoadBalancerTypes...)
		return resp, nil
	})
	if err != nil {
		return nil, err
	}

	return allLoadBalancerTypes, nil
}
