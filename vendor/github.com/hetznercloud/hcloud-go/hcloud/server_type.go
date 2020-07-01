package hcloud

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/hetznercloud/hcloud-go/hcloud/schema"
)

// ServerType represents a server type in the Hetzner Cloud.
type ServerType struct {
	ID          int
	Name        string
	Description string
	Cores       int
	Memory      float32
	Disk        int
	StorageType StorageType
	CPUType     CPUType
	Pricings    []ServerTypeLocationPricing
}

// StorageType specifies the type of storage.
type StorageType string

const (
	// StorageTypeLocal is the type for local storage.
	StorageTypeLocal StorageType = "local"

	// StorageTypeCeph is the type for remote storage.
	StorageTypeCeph StorageType = "ceph"
)

// CPUType specifies the type of the CPU.
type CPUType string

const (
	// CPUTypeShared is the type for shared CPU.
	CPUTypeShared CPUType = "shared"

	//CPUTypeDedicated is the type for dedicated CPU.
	CPUTypeDedicated CPUType = "dedicated"
)

// ServerTypeClient is a client for the server types API.
type ServerTypeClient struct {
	client *Client
}

// GetByID retrieves a server type by its ID. If the server type does not exist, nil is returned.
func (c *ServerTypeClient) GetByID(ctx context.Context, id int) (*ServerType, *Response, error) {
	req, err := c.client.NewRequest(ctx, "GET", fmt.Sprintf("/server_types/%d", id), nil)
	if err != nil {
		return nil, nil, err
	}

	var body schema.ServerTypeGetResponse
	resp, err := c.client.Do(req, &body)
	if err != nil {
		if IsError(err, ErrorCodeNotFound) {
			return nil, resp, nil
		}
		return nil, nil, err
	}
	return ServerTypeFromSchema(body.ServerType), resp, nil
}

// GetByName retrieves a server type by its name. If the server type does not exist, nil is returned.
func (c *ServerTypeClient) GetByName(ctx context.Context, name string) (*ServerType, *Response, error) {
	if name == "" {
		return nil, nil, nil
	}
	serverTypes, response, err := c.List(ctx, ServerTypeListOpts{Name: name})
	if len(serverTypes) == 0 {
		return nil, response, err
	}
	return serverTypes[0], response, err
}

// Get retrieves a server type by its ID if the input can be parsed as an integer, otherwise it
// retrieves a server type by its name. If the server type does not exist, nil is returned.
func (c *ServerTypeClient) Get(ctx context.Context, idOrName string) (*ServerType, *Response, error) {
	if id, err := strconv.Atoi(idOrName); err == nil {
		return c.GetByID(ctx, int(id))
	}
	return c.GetByName(ctx, idOrName)
}

// ServerTypeListOpts specifies options for listing server types.
type ServerTypeListOpts struct {
	ListOpts
	Name string
}

func (l ServerTypeListOpts) values() url.Values {
	vals := l.ListOpts.values()
	if l.Name != "" {
		vals.Add("name", l.Name)
	}
	return vals
}

// List returns a list of server types for a specific page.
//
// Please note that filters specified in opts are not taken into account
// when their value corresponds to their zero value or when they are empty.
func (c *ServerTypeClient) List(ctx context.Context, opts ServerTypeListOpts) ([]*ServerType, *Response, error) {
	path := "/server_types?" + opts.values().Encode()
	req, err := c.client.NewRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, nil, err
	}

	var body schema.ServerTypeListResponse
	resp, err := c.client.Do(req, &body)
	if err != nil {
		return nil, nil, err
	}
	serverTypes := make([]*ServerType, 0, len(body.ServerTypes))
	for _, s := range body.ServerTypes {
		serverTypes = append(serverTypes, ServerTypeFromSchema(s))
	}
	return serverTypes, resp, nil
}

// All returns all server types.
func (c *ServerTypeClient) All(ctx context.Context) ([]*ServerType, error) {
	allServerTypes := []*ServerType{}

	opts := ServerTypeListOpts{}
	opts.PerPage = 50

	_, err := c.client.all(func(page int) (*Response, error) {
		opts.Page = page
		serverTypes, resp, err := c.List(ctx, opts)
		if err != nil {
			return resp, err
		}
		allServerTypes = append(allServerTypes, serverTypes...)
		return resp, nil
	})
	if err != nil {
		return nil, err
	}

	return allServerTypes, nil
}
