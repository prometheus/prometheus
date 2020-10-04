package godo

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const cdnBasePath = "v2/cdn/endpoints"

// CDNService is an interface for managing Spaces CDN with the DigitalOcean API.
type CDNService interface {
	List(context.Context, *ListOptions) ([]CDN, *Response, error)
	Get(context.Context, string) (*CDN, *Response, error)
	Create(context.Context, *CDNCreateRequest) (*CDN, *Response, error)
	UpdateTTL(context.Context, string, *CDNUpdateTTLRequest) (*CDN, *Response, error)
	UpdateCustomDomain(context.Context, string, *CDNUpdateCustomDomainRequest) (*CDN, *Response, error)
	FlushCache(context.Context, string, *CDNFlushCacheRequest) (*Response, error)
	Delete(context.Context, string) (*Response, error)
}

// CDNServiceOp handles communication with the CDN related methods of the
// DigitalOcean API.
type CDNServiceOp struct {
	client *Client
}

var _ CDNService = &CDNServiceOp{}

// CDN represents a DigitalOcean CDN
type CDN struct {
	ID            string    `json:"id"`
	Origin        string    `json:"origin"`
	Endpoint      string    `json:"endpoint"`
	CreatedAt     time.Time `json:"created_at"`
	TTL           uint32    `json:"ttl"`
	CertificateID string    `json:"certificate_id,omitempty"`
	CustomDomain  string    `json:"custom_domain,omitempty"`
}

// CDNRoot represents a response from the DigitalOcean API
type cdnRoot struct {
	Endpoint *CDN `json:"endpoint"`
}

type cdnsRoot struct {
	Endpoints []CDN  `json:"endpoints"`
	Links     *Links `json:"links"`
	Meta      *Meta  `json:"meta"`
}

// CDNCreateRequest represents a request to create a CDN.
type CDNCreateRequest struct {
	Origin        string `json:"origin"`
	TTL           uint32 `json:"ttl"`
	CustomDomain  string `json:"custom_domain,omitempty"`
	CertificateID string `json:"certificate_id,omitempty"`
}

// CDNUpdateTTLRequest represents a request to update the ttl of a CDN.
type CDNUpdateTTLRequest struct {
	TTL uint32 `json:"ttl"`
}

// CDNUpdateCustomDomainRequest represents a request to update the custom domain of a CDN.
type CDNUpdateCustomDomainRequest struct {
	CustomDomain  string `json:"custom_domain"`
	CertificateID string `json:"certificate_id"`
}

// CDNFlushCacheRequest represents a request to flush cache of a CDN.
type CDNFlushCacheRequest struct {
	Files []string `json:"files"`
}

// List all CDN endpoints
func (c CDNServiceOp) List(ctx context.Context, opt *ListOptions) ([]CDN, *Response, error) {
	path, err := addOptions(cdnBasePath, opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := c.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	root := new(cdnsRoot)
	resp, err := c.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	if l := root.Links; l != nil {
		resp.Links = l
	}
	if m := root.Meta; m != nil {
		resp.Meta = m
	}

	return root.Endpoints, resp, err
}

// Get individual CDN. It requires a non-empty cdn id.
func (c CDNServiceOp) Get(ctx context.Context, id string) (*CDN, *Response, error) {
	if len(id) == 0 {
		return nil, nil, NewArgError("id", "cannot be an empty string")
	}

	path := fmt.Sprintf("%s/%s", cdnBasePath, id)

	req, err := c.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	root := new(cdnRoot)
	resp, err := c.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	return root.Endpoint, resp, err
}

// Create a new CDN
func (c CDNServiceOp) Create(ctx context.Context, createRequest *CDNCreateRequest) (*CDN, *Response, error) {
	if createRequest == nil {
		return nil, nil, NewArgError("createRequest", "cannot be nil")
	}

	req, err := c.client.NewRequest(ctx, http.MethodPost, cdnBasePath, createRequest)
	if err != nil {
		return nil, nil, err
	}

	root := new(cdnRoot)
	resp, err := c.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	return root.Endpoint, resp, err
}

// UpdateTTL updates the ttl of an individual CDN
func (c CDNServiceOp) UpdateTTL(ctx context.Context, id string, updateRequest *CDNUpdateTTLRequest) (*CDN, *Response, error) {
	return c.update(ctx, id, updateRequest)
}

// UpdateCustomDomain sets or removes the custom domain of an individual CDN
func (c CDNServiceOp) UpdateCustomDomain(ctx context.Context, id string, updateRequest *CDNUpdateCustomDomainRequest) (*CDN, *Response, error) {
	return c.update(ctx, id, updateRequest)
}

func (c CDNServiceOp) update(ctx context.Context, id string, updateRequest interface{}) (*CDN, *Response, error) {
	if updateRequest == nil {
		return nil, nil, NewArgError("updateRequest", "cannot be nil")
	}

	if len(id) == 0 {
		return nil, nil, NewArgError("id", "cannot be an empty string")
	}
	path := fmt.Sprintf("%s/%s", cdnBasePath, id)

	req, err := c.client.NewRequest(ctx, http.MethodPut, path, updateRequest)
	if err != nil {
		return nil, nil, err
	}

	root := new(cdnRoot)
	resp, err := c.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	return root.Endpoint, resp, err
}

// FlushCache flushes the cache of an individual CDN. Requires a non-empty slice of file paths and/or wildcards
func (c CDNServiceOp) FlushCache(ctx context.Context, id string, flushCacheRequest *CDNFlushCacheRequest) (*Response, error) {
	if flushCacheRequest == nil {
		return nil, NewArgError("flushCacheRequest", "cannot be nil")
	}

	if len(id) == 0 {
		return nil, NewArgError("id", "cannot be an empty string")
	}

	path := fmt.Sprintf("%s/%s/cache", cdnBasePath, id)

	req, err := c.client.NewRequest(ctx, http.MethodDelete, path, flushCacheRequest)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(ctx, req, nil)

	return resp, err
}

// Delete an individual CDN
func (c CDNServiceOp) Delete(ctx context.Context, id string) (*Response, error) {
	if len(id) == 0 {
		return nil, NewArgError("id", "cannot be an empty string")
	}

	path := fmt.Sprintf("%s/%s", cdnBasePath, id)

	req, err := c.client.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(ctx, req, nil)

	return resp, err
}
