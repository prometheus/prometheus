package hcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/hetznercloud/hcloud-go/hcloud/schema"
)

// NetworkZone specifies a network zone.
type NetworkZone string

// List of available Network Zones.
const (
	NetworkZoneEUCentral NetworkZone = "eu-central"
)

// NetworkSubnetType specifies a type of a subnet.
type NetworkSubnetType string

// List of available network subnet types.
const (
	NetworkSubnetTypeCloud  NetworkSubnetType = "cloud"
	NetworkSubnetTypeServer NetworkSubnetType = "server"
)

// Network represents a network in the Hetzner Cloud.
type Network struct {
	ID         int
	Name       string
	Created    time.Time
	IPRange    *net.IPNet
	Subnets    []NetworkSubnet
	Routes     []NetworkRoute
	Servers    []*Server
	Protection NetworkProtection
	Labels     map[string]string
}

// NetworkSubnet represents a subnet of a network in the Hetzner Cloud.
type NetworkSubnet struct {
	Type        NetworkSubnetType
	IPRange     *net.IPNet
	NetworkZone NetworkZone
	Gateway     net.IP
}

// NetworkRoute represents a route of a network.
type NetworkRoute struct {
	Destination *net.IPNet
	Gateway     net.IP
}

// NetworkProtection represents the protection level of a network.
type NetworkProtection struct {
	Delete bool
}

// NetworkClient is a client for the network API.
type NetworkClient struct {
	client *Client
}

// GetByID retrieves a network by its ID. If the network does not exist, nil is returned.
func (c *NetworkClient) GetByID(ctx context.Context, id int) (*Network, *Response, error) {
	req, err := c.client.NewRequest(ctx, "GET", fmt.Sprintf("/networks/%d", id), nil)
	if err != nil {
		return nil, nil, err
	}

	var body schema.NetworkGetResponse
	resp, err := c.client.Do(req, &body)
	if err != nil {
		if IsError(err, ErrorCodeNotFound) {
			return nil, resp, nil
		}
		return nil, nil, err
	}
	return NetworkFromSchema(body.Network), resp, nil
}

// GetByName retrieves a network by its name. If the network does not exist, nil is returned.
func (c *NetworkClient) GetByName(ctx context.Context, name string) (*Network, *Response, error) {
	if name == "" {
		return nil, nil, nil
	}
	Networks, response, err := c.List(ctx, NetworkListOpts{Name: name})
	if len(Networks) == 0 {
		return nil, response, err
	}
	return Networks[0], response, err
}

// Get retrieves a network by its ID if the input can be parsed as an integer, otherwise it
// retrieves a network by its name. If the network does not exist, nil is returned.
func (c *NetworkClient) Get(ctx context.Context, idOrName string) (*Network, *Response, error) {
	if id, err := strconv.Atoi(idOrName); err == nil {
		return c.GetByID(ctx, int(id))
	}
	return c.GetByName(ctx, idOrName)
}

// NetworkListOpts specifies options for listing networks.
type NetworkListOpts struct {
	ListOpts
	Name string
}

func (l NetworkListOpts) values() url.Values {
	vals := l.ListOpts.values()
	if l.Name != "" {
		vals.Add("name", l.Name)
	}
	return vals
}

// List returns a list of networks for a specific page.
//
// Please note that filters specified in opts are not taken into account
// when their value corresponds to their zero value or when they are empty.
func (c *NetworkClient) List(ctx context.Context, opts NetworkListOpts) ([]*Network, *Response, error) {
	path := "/networks?" + opts.values().Encode()
	req, err := c.client.NewRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, nil, err
	}

	var body schema.NetworkListResponse
	resp, err := c.client.Do(req, &body)
	if err != nil {
		return nil, nil, err
	}
	Networks := make([]*Network, 0, len(body.Networks))
	for _, s := range body.Networks {
		Networks = append(Networks, NetworkFromSchema(s))
	}
	return Networks, resp, nil
}

// All returns all networks.
func (c *NetworkClient) All(ctx context.Context) ([]*Network, error) {
	return c.AllWithOpts(ctx, NetworkListOpts{ListOpts: ListOpts{PerPage: 50}})
}

// AllWithOpts returns all networks for the given options.
func (c *NetworkClient) AllWithOpts(ctx context.Context, opts NetworkListOpts) ([]*Network, error) {
	var allNetworks []*Network

	_, err := c.client.all(func(page int) (*Response, error) {
		opts.Page = page
		Networks, resp, err := c.List(ctx, opts)
		if err != nil {
			return resp, err
		}
		allNetworks = append(allNetworks, Networks...)
		return resp, nil
	})
	if err != nil {
		return nil, err
	}

	return allNetworks, nil
}

// Delete deletes a network.
func (c *NetworkClient) Delete(ctx context.Context, network *Network) (*Response, error) {
	req, err := c.client.NewRequest(ctx, "DELETE", fmt.Sprintf("/networks/%d", network.ID), nil)
	if err != nil {
		return nil, err
	}
	return c.client.Do(req, nil)
}

// NetworkUpdateOpts specifies options for updating a network.
type NetworkUpdateOpts struct {
	Name   string
	Labels map[string]string
}

// Update updates a network.
func (c *NetworkClient) Update(ctx context.Context, network *Network, opts NetworkUpdateOpts) (*Network, *Response, error) {
	reqBody := schema.NetworkUpdateRequest{
		Name: opts.Name,
	}
	if opts.Labels != nil {
		reqBody.Labels = &opts.Labels
	}
	reqBodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}

	path := fmt.Sprintf("/networks/%d", network.ID)
	req, err := c.client.NewRequest(ctx, "PUT", path, bytes.NewReader(reqBodyData))
	if err != nil {
		return nil, nil, err
	}

	respBody := schema.NetworkUpdateResponse{}
	resp, err := c.client.Do(req, &respBody)
	if err != nil {
		return nil, resp, err
	}
	return NetworkFromSchema(respBody.Network), resp, nil
}

// NetworkCreateOpts specifies options for creating a new network.
type NetworkCreateOpts struct {
	Name    string
	IPRange *net.IPNet
	Subnets []NetworkSubnet
	Routes  []NetworkRoute
	Labels  map[string]string
}

// Validate checks if options are valid.
func (o NetworkCreateOpts) Validate() error {
	if o.Name == "" {
		return errors.New("missing name")
	}
	if o.IPRange == nil || o.IPRange.String() == "" {
		return errors.New("missing IP range")
	}
	return nil
}

// Create creates a new network.
func (c *NetworkClient) Create(ctx context.Context, opts NetworkCreateOpts) (*Network, *Response, error) {
	if err := opts.Validate(); err != nil {
		return nil, nil, err
	}
	reqBody := schema.NetworkCreateRequest{
		Name:    opts.Name,
		IPRange: opts.IPRange.String(),
	}
	for _, subnet := range opts.Subnets {
		reqBody.Subnets = append(reqBody.Subnets, schema.NetworkSubnet{
			Type:        string(subnet.Type),
			IPRange:     subnet.IPRange.String(),
			NetworkZone: string(subnet.NetworkZone),
		})
	}
	for _, route := range opts.Routes {
		reqBody.Routes = append(reqBody.Routes, schema.NetworkRoute{
			Destination: route.Destination.String(),
			Gateway:     route.Gateway.String(),
		})
	}
	if opts.Labels != nil {
		reqBody.Labels = &opts.Labels
	}
	reqBodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}
	req, err := c.client.NewRequest(ctx, "POST", "/networks", bytes.NewReader(reqBodyData))
	if err != nil {
		return nil, nil, err
	}

	respBody := schema.NetworkCreateResponse{}
	resp, err := c.client.Do(req, &respBody)
	if err != nil {
		return nil, resp, err
	}
	return NetworkFromSchema(respBody.Network), resp, nil
}

// NetworkChangeIPRangeOpts specifies options for changing the IP range of a network.
type NetworkChangeIPRangeOpts struct {
	IPRange *net.IPNet
}

// ChangeIPRange changes the IP range of a network.
func (c *NetworkClient) ChangeIPRange(ctx context.Context, network *Network, opts NetworkChangeIPRangeOpts) (*Action, *Response, error) {
	reqBody := schema.NetworkActionChangeIPRangeRequest{
		IPRange: opts.IPRange.String(),
	}
	reqBodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}

	path := fmt.Sprintf("/networks/%d/actions/change_ip_range", network.ID)
	req, err := c.client.NewRequest(ctx, "POST", path, bytes.NewReader(reqBodyData))
	if err != nil {
		return nil, nil, err
	}

	respBody := schema.NetworkActionChangeIPRangeResponse{}
	resp, err := c.client.Do(req, &respBody)
	if err != nil {
		return nil, resp, err
	}
	return ActionFromSchema(respBody.Action), resp, nil
}

// NetworkAddSubnetOpts specifies options for adding a subnet to a network.
type NetworkAddSubnetOpts struct {
	Subnet NetworkSubnet
}

// AddSubnet adds a subnet to a network.
func (c *NetworkClient) AddSubnet(ctx context.Context, network *Network, opts NetworkAddSubnetOpts) (*Action, *Response, error) {
	reqBody := schema.NetworkActionAddSubnetRequest{
		Type:        string(opts.Subnet.Type),
		NetworkZone: string(opts.Subnet.NetworkZone),
	}
	if opts.Subnet.IPRange != nil {
		reqBody.IPRange = opts.Subnet.IPRange.String()
	}
	reqBodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}

	path := fmt.Sprintf("/networks/%d/actions/add_subnet", network.ID)
	req, err := c.client.NewRequest(ctx, "POST", path, bytes.NewReader(reqBodyData))
	if err != nil {
		return nil, nil, err
	}

	respBody := schema.NetworkActionAddSubnetResponse{}
	resp, err := c.client.Do(req, &respBody)
	if err != nil {
		return nil, resp, err
	}
	return ActionFromSchema(respBody.Action), resp, nil
}

// NetworkDeleteSubnetOpts specifies options for deleting a subnet from a network.
type NetworkDeleteSubnetOpts struct {
	Subnet NetworkSubnet
}

// DeleteSubnet deletes a subnet from a network.
func (c *NetworkClient) DeleteSubnet(ctx context.Context, network *Network, opts NetworkDeleteSubnetOpts) (*Action, *Response, error) {
	reqBody := schema.NetworkActionDeleteSubnetRequest{
		IPRange: opts.Subnet.IPRange.String(),
	}
	reqBodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}

	path := fmt.Sprintf("/networks/%d/actions/delete_subnet", network.ID)
	req, err := c.client.NewRequest(ctx, "POST", path, bytes.NewReader(reqBodyData))
	if err != nil {
		return nil, nil, err
	}

	respBody := schema.NetworkActionDeleteSubnetResponse{}
	resp, err := c.client.Do(req, &respBody)
	if err != nil {
		return nil, resp, err
	}
	return ActionFromSchema(respBody.Action), resp, nil
}

// NetworkAddRouteOpts specifies options for adding a route to a network.
type NetworkAddRouteOpts struct {
	Route NetworkRoute
}

// AddRoute adds a route to a network.
func (c *NetworkClient) AddRoute(ctx context.Context, network *Network, opts NetworkAddRouteOpts) (*Action, *Response, error) {
	reqBody := schema.NetworkActionAddRouteRequest{
		Destination: opts.Route.Destination.String(),
		Gateway:     opts.Route.Gateway.String(),
	}
	reqBodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}

	path := fmt.Sprintf("/networks/%d/actions/add_route", network.ID)
	req, err := c.client.NewRequest(ctx, "POST", path, bytes.NewReader(reqBodyData))
	if err != nil {
		return nil, nil, err
	}

	respBody := schema.NetworkActionAddSubnetResponse{}
	resp, err := c.client.Do(req, &respBody)
	if err != nil {
		return nil, resp, err
	}
	return ActionFromSchema(respBody.Action), resp, nil
}

// NetworkDeleteRouteOpts specifies options for deleting a route from a network.
type NetworkDeleteRouteOpts struct {
	Route NetworkRoute
}

// DeleteRoute deletes a route from a network.
func (c *NetworkClient) DeleteRoute(ctx context.Context, network *Network, opts NetworkDeleteRouteOpts) (*Action, *Response, error) {
	reqBody := schema.NetworkActionDeleteRouteRequest{
		Destination: opts.Route.Destination.String(),
		Gateway:     opts.Route.Gateway.String(),
	}
	reqBodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}

	path := fmt.Sprintf("/networks/%d/actions/delete_route", network.ID)
	req, err := c.client.NewRequest(ctx, "POST", path, bytes.NewReader(reqBodyData))
	if err != nil {
		return nil, nil, err
	}

	respBody := schema.NetworkActionDeleteSubnetResponse{}
	resp, err := c.client.Do(req, &respBody)
	if err != nil {
		return nil, resp, err
	}
	return ActionFromSchema(respBody.Action), resp, nil
}

// NetworkChangeProtectionOpts specifies options for changing the resource protection level of a network.
type NetworkChangeProtectionOpts struct {
	Delete *bool
}

// ChangeProtection changes the resource protection level of a network.
func (c *NetworkClient) ChangeProtection(ctx context.Context, network *Network, opts NetworkChangeProtectionOpts) (*Action, *Response, error) {
	reqBody := schema.NetworkActionChangeProtectionRequest{
		Delete: opts.Delete,
	}
	reqBodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}

	path := fmt.Sprintf("/networks/%d/actions/change_protection", network.ID)
	req, err := c.client.NewRequest(ctx, "POST", path, bytes.NewReader(reqBodyData))
	if err != nil {
		return nil, nil, err
	}

	respBody := schema.NetworkActionChangeProtectionResponse{}
	resp, err := c.client.Do(req, &respBody)
	if err != nil {
		return nil, resp, err
	}
	return ActionFromSchema(respBody.Action), resp, err
}
