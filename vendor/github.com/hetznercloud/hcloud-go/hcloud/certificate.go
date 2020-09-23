package hcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/hetznercloud/hcloud-go/hcloud/schema"
)

// Certificate represents an certificate in the Hetzner Cloud.
type Certificate struct {
	ID             int
	Name           string
	Labels         map[string]string
	Certificate    string
	Created        time.Time
	NotValidBefore time.Time
	NotValidAfter  time.Time
	DomainNames    []string
	Fingerprint    string
}

// CertificateClient is a client for the Certificates API.
type CertificateClient struct {
	client *Client
}

// GetByID retrieves a Certificate by its ID. If the Certificate does not exist, nil is returned.
func (c *CertificateClient) GetByID(ctx context.Context, id int) (*Certificate, *Response, error) {
	req, err := c.client.NewRequest(ctx, "GET", fmt.Sprintf("/certificates/%d", id), nil)
	if err != nil {
		return nil, nil, err
	}

	var body schema.CertificateGetResponse
	resp, err := c.client.Do(req, &body)
	if err != nil {
		if IsError(err, ErrorCodeNotFound) {
			return nil, resp, nil
		}
		return nil, nil, err
	}
	return CertificateFromSchema(body.Certificate), resp, nil
}

// GetByName retrieves a Certificate by its name. If the Certificate does not exist, nil is returned.
func (c *CertificateClient) GetByName(ctx context.Context, name string) (*Certificate, *Response, error) {
	if name == "" {
		return nil, nil, nil
	}
	Certificate, response, err := c.List(ctx, CertificateListOpts{Name: name})
	if len(Certificate) == 0 {
		return nil, response, err
	}
	return Certificate[0], response, err
}

// Get retrieves a Certificate by its ID if the input can be parsed as an integer, otherwise it
// retrieves a Certificate by its name. If the Certificate does not exist, nil is returned.
func (c *CertificateClient) Get(ctx context.Context, idOrName string) (*Certificate, *Response, error) {
	if id, err := strconv.Atoi(idOrName); err == nil {
		return c.GetByID(ctx, int(id))
	}
	return c.GetByName(ctx, idOrName)
}

// CertificateListOpts specifies options for listing Certificates.
type CertificateListOpts struct {
	ListOpts
	Name string
}

func (l CertificateListOpts) values() url.Values {
	vals := l.ListOpts.values()
	if l.Name != "" {
		vals.Add("name", l.Name)
	}
	return vals
}

// List returns a list of Certificates for a specific page.
//
// Please note that filters specified in opts are not taken into account
// when their value corresponds to their zero value or when they are empty.
func (c *CertificateClient) List(ctx context.Context, opts CertificateListOpts) ([]*Certificate, *Response, error) {
	path := "/certificates?" + opts.values().Encode()
	req, err := c.client.NewRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, nil, err
	}

	var body schema.CertificateListResponse
	resp, err := c.client.Do(req, &body)
	if err != nil {
		return nil, nil, err
	}
	Certificates := make([]*Certificate, 0, len(body.Certificates))
	for _, s := range body.Certificates {
		Certificates = append(Certificates, CertificateFromSchema(s))
	}
	return Certificates, resp, nil
}

// All returns all Certificates.
func (c *CertificateClient) All(ctx context.Context) ([]*Certificate, error) {
	allCertificates := []*Certificate{}

	opts := CertificateListOpts{}
	opts.PerPage = 50

	_, err := c.client.all(func(page int) (*Response, error) {
		opts.Page = page
		Certificate, resp, err := c.List(ctx, opts)
		if err != nil {
			return resp, err
		}
		allCertificates = append(allCertificates, Certificate...)
		return resp, nil
	})
	if err != nil {
		return nil, err
	}

	return allCertificates, nil
}

// AllWithOpts returns all Certificates for the given options.
func (c *CertificateClient) AllWithOpts(ctx context.Context, opts CertificateListOpts) ([]*Certificate, error) {
	var allCertificates []*Certificate

	_, err := c.client.all(func(page int) (*Response, error) {
		opts.Page = page
		Certificates, resp, err := c.List(ctx, opts)
		if err != nil {
			return resp, err
		}
		allCertificates = append(allCertificates, Certificates...)
		return resp, nil
	})
	if err != nil {
		return nil, err
	}

	return allCertificates, nil
}

// CertificateCreateOpts specifies options for creating a new Certificate.
type CertificateCreateOpts struct {
	Name        string
	Certificate string
	PrivateKey  string
	Labels      map[string]string
}

// Validate checks if options are valid.
func (o CertificateCreateOpts) Validate() error {
	if o.Name == "" {
		return errors.New("missing name")
	}
	if o.Certificate == "" {
		return errors.New("missing certificate")
	}
	if o.PrivateKey == "" {
		return errors.New("missing private key")
	}
	return nil
}

// Create creates a new certificate.
func (c *CertificateClient) Create(ctx context.Context, opts CertificateCreateOpts) (*Certificate, *Response, error) {
	if err := opts.Validate(); err != nil {
		return nil, nil, err
	}
	reqBody := schema.CertificateCreateRequest{
		Name:        opts.Name,
		Certificate: opts.Certificate,
		PrivateKey:  opts.PrivateKey,
	}
	if opts.Labels != nil {
		reqBody.Labels = &opts.Labels
	}
	reqBodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}
	req, err := c.client.NewRequest(ctx, "POST", "/certificates", bytes.NewReader(reqBodyData))
	if err != nil {
		return nil, nil, err
	}

	respBody := schema.CertificateCreateResponse{}
	resp, err := c.client.Do(req, &respBody)
	if err != nil {
		return nil, resp, err
	}
	return CertificateFromSchema(respBody.Certificate), resp, nil
}

// CertificateUpdateOpts specifies options for updating a Certificate.
type CertificateUpdateOpts struct {
	Name   string
	Labels map[string]string
}

// Update updates a Certificate.
func (c *CertificateClient) Update(ctx context.Context, certificate *Certificate, opts CertificateUpdateOpts) (*Certificate, *Response, error) {
	reqBody := schema.CertificateUpdateRequest{}
	if opts.Name != "" {
		reqBody.Name = &opts.Name
	}
	if opts.Labels != nil {
		reqBody.Labels = &opts.Labels
	}
	reqBodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}

	path := fmt.Sprintf("/certificates/%d", certificate.ID)
	req, err := c.client.NewRequest(ctx, "PUT", path, bytes.NewReader(reqBodyData))
	if err != nil {
		return nil, nil, err
	}

	respBody := schema.CertificateUpdateResponse{}
	resp, err := c.client.Do(req, &respBody)
	if err != nil {
		return nil, resp, err
	}
	return CertificateFromSchema(respBody.Certificate), resp, nil
}

// Delete deletes a certificate.
func (c *CertificateClient) Delete(ctx context.Context, certificate *Certificate) (*Response, error) {
	req, err := c.client.NewRequest(ctx, "DELETE", fmt.Sprintf("/certificates/%d", certificate.ID), nil)
	if err != nil {
		return nil, err
	}
	return c.client.Do(req, nil)
}
