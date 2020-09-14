package hcloud

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/hetznercloud/hcloud-go/hcloud/schema"
)

// ISO represents an ISO image in the Hetzner Cloud.
type ISO struct {
	ID          int
	Name        string
	Description string
	Type        ISOType
	Deprecated  time.Time
}

// IsDeprecated returns true if the ISO is deprecated
func (iso *ISO) IsDeprecated() bool {
	return !iso.Deprecated.IsZero()
}

// ISOType specifies the type of an ISO image.
type ISOType string

const (
	// ISOTypePublic is the type of a public ISO image.
	ISOTypePublic ISOType = "public"

	// ISOTypePrivate is the type of a private ISO image.
	ISOTypePrivate ISOType = "private"
)

// ISOClient is a client for the ISO API.
type ISOClient struct {
	client *Client
}

// GetByID retrieves an ISO by its ID.
func (c *ISOClient) GetByID(ctx context.Context, id int) (*ISO, *Response, error) {
	req, err := c.client.NewRequest(ctx, "GET", fmt.Sprintf("/isos/%d", id), nil)
	if err != nil {
		return nil, nil, err
	}

	var body schema.ISOGetResponse
	resp, err := c.client.Do(req, &body)
	if err != nil {
		if IsError(err, ErrorCodeNotFound) {
			return nil, resp, nil
		}
		return nil, resp, err
	}
	return ISOFromSchema(body.ISO), resp, nil
}

// GetByName retrieves an ISO by its name.
func (c *ISOClient) GetByName(ctx context.Context, name string) (*ISO, *Response, error) {
	if name == "" {
		return nil, nil, nil
	}
	isos, response, err := c.List(ctx, ISOListOpts{Name: name})
	if len(isos) == 0 {
		return nil, response, err
	}
	return isos[0], response, err
}

// Get retrieves an ISO by its ID if the input can be parsed as an integer, otherwise it retrieves an ISO by its name.
func (c *ISOClient) Get(ctx context.Context, idOrName string) (*ISO, *Response, error) {
	if id, err := strconv.Atoi(idOrName); err == nil {
		return c.GetByID(ctx, int(id))
	}
	return c.GetByName(ctx, idOrName)
}

// ISOListOpts specifies options for listing isos.
type ISOListOpts struct {
	ListOpts
	Name string
}

func (l ISOListOpts) values() url.Values {
	vals := l.ListOpts.values()
	if l.Name != "" {
		vals.Add("name", l.Name)
	}
	return vals
}

// List returns a list of ISOs for a specific page.
//
// Please note that filters specified in opts are not taken into account
// when their value corresponds to their zero value or when they are empty.
func (c *ISOClient) List(ctx context.Context, opts ISOListOpts) ([]*ISO, *Response, error) {
	path := "/isos?" + opts.values().Encode()
	req, err := c.client.NewRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, nil, err
	}

	var body schema.ISOListResponse
	resp, err := c.client.Do(req, &body)
	if err != nil {
		return nil, nil, err
	}
	isos := make([]*ISO, 0, len(body.ISOs))
	for _, i := range body.ISOs {
		isos = append(isos, ISOFromSchema(i))
	}
	return isos, resp, nil
}

// All returns all ISOs.
func (c *ISOClient) All(ctx context.Context) ([]*ISO, error) {
	allISOs := []*ISO{}

	opts := ISOListOpts{}
	opts.PerPage = 50

	_, err := c.client.all(func(page int) (*Response, error) {
		opts.Page = page
		isos, resp, err := c.List(ctx, opts)
		if err != nil {
			return resp, err
		}
		allISOs = append(allISOs, isos...)
		return resp, nil
	})
	if err != nil {
		return nil, err
	}

	return allISOs, nil
}
