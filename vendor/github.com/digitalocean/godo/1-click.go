package godo

import (
	"context"
	"fmt"
	"net/http"
)

const oneClickBasePath = "v2/1-clicks"

// OneClickService is an interface for interacting with 1-clicks with the
// DigitalOcean API.
// See: https://developers.digitalocean.com/documentation/v2/#1-click-applications
type OneClickService interface {
	List(context.Context, string) ([]*OneClick, *Response, error)
}

var _ OneClickService = &OneClickServiceOp{}

// OneClickServiceOp interfaces with 1-click endpoints in the DigitalOcean API.
type OneClickServiceOp struct {
	client *Client
}

// OneClick is the structure of a 1-click
type OneClick struct {
	Slug string `json:"slug"`
	Type string `json:"type"`
}

// OneClicksRoot is the root of the json payload that contains a list of 1-clicks
type OneClicksRoot struct {
	List []*OneClick `json:"1_clicks"`
}

// List returns a list of the available 1-click applications.
func (ocs *OneClickServiceOp) List(ctx context.Context, oneClickType string) ([]*OneClick, *Response, error) {
	path := fmt.Sprintf(`%s?type=%s`, oneClickBasePath, oneClickType)

	req, err := ocs.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	root := new(OneClicksRoot)
	resp, err := ocs.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	return root.List, resp, nil
}
