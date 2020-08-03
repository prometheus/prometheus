package godo

import (
	"context"
	"fmt"
	"net/http"
)

const domainsBasePath = "v2/domains"

// DomainsService is an interface for managing DNS with the DigitalOcean API.
// See: https://developers.digitalocean.com/documentation/v2#domains and
// https://developers.digitalocean.com/documentation/v2#domain-records
type DomainsService interface {
	List(context.Context, *ListOptions) ([]Domain, *Response, error)
	Get(context.Context, string) (*Domain, *Response, error)
	Create(context.Context, *DomainCreateRequest) (*Domain, *Response, error)
	Delete(context.Context, string) (*Response, error)

	Records(context.Context, string, *ListOptions) ([]DomainRecord, *Response, error)
	RecordsByType(context.Context, string, string, *ListOptions) ([]DomainRecord, *Response, error)
	RecordsByName(context.Context, string, string, *ListOptions) ([]DomainRecord, *Response, error)
	RecordsByTypeAndName(context.Context, string, string, string, *ListOptions) ([]DomainRecord, *Response, error)
	Record(context.Context, string, int) (*DomainRecord, *Response, error)
	DeleteRecord(context.Context, string, int) (*Response, error)
	EditRecord(context.Context, string, int, *DomainRecordEditRequest) (*DomainRecord, *Response, error)
	CreateRecord(context.Context, string, *DomainRecordEditRequest) (*DomainRecord, *Response, error)
}

// DomainsServiceOp handles communication with the domain related methods of the
// DigitalOcean API.
type DomainsServiceOp struct {
	client *Client
}

var _ DomainsService = &DomainsServiceOp{}

// Domain represents a DigitalOcean domain
type Domain struct {
	Name     string `json:"name"`
	TTL      int    `json:"ttl"`
	ZoneFile string `json:"zone_file"`
}

// domainRoot represents a response from the DigitalOcean API
type domainRoot struct {
	Domain *Domain `json:"domain"`
}

type domainsRoot struct {
	Domains []Domain `json:"domains"`
	Links   *Links   `json:"links"`
	Meta    *Meta    `json:"meta"`
}

// DomainCreateRequest respresents a request to create a domain.
type DomainCreateRequest struct {
	Name      string `json:"name"`
	IPAddress string `json:"ip_address,omitempty"`
}

// DomainRecordRoot is the root of an individual Domain Record response
type domainRecordRoot struct {
	DomainRecord *DomainRecord `json:"domain_record"`
}

// DomainRecordsRoot is the root of a group of Domain Record responses
type domainRecordsRoot struct {
	DomainRecords []DomainRecord `json:"domain_records"`
	Links         *Links         `json:"links"`
}

// DomainRecord represents a DigitalOcean DomainRecord
type DomainRecord struct {
	ID       int    `json:"id,float64,omitempty"`
	Type     string `json:"type,omitempty"`
	Name     string `json:"name,omitempty"`
	Data     string `json:"data,omitempty"`
	Priority int    `json:"priority"`
	Port     int    `json:"port,omitempty"`
	TTL      int    `json:"ttl,omitempty"`
	Weight   int    `json:"weight"`
	Flags    int    `json:"flags"`
	Tag      string `json:"tag,omitempty"`
}

// DomainRecordEditRequest represents a request to update a domain record.
type DomainRecordEditRequest struct {
	Type     string `json:"type,omitempty"`
	Name     string `json:"name,omitempty"`
	Data     string `json:"data,omitempty"`
	Priority int    `json:"priority"`
	Port     int    `json:"port,omitempty"`
	TTL      int    `json:"ttl,omitempty"`
	Weight   int    `json:"weight"`
	Flags    int    `json:"flags"`
	Tag      string `json:"tag,omitempty"`
}

func (d Domain) String() string {
	return Stringify(d)
}

func (d Domain) URN() string {
	return ToURN("Domain", d.Name)
}

// List all domains.
func (s DomainsServiceOp) List(ctx context.Context, opt *ListOptions) ([]Domain, *Response, error) {
	path := domainsBasePath
	path, err := addOptions(path, opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	root := new(domainsRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	if l := root.Links; l != nil {
		resp.Links = l
	}
	if m := root.Meta; m != nil {
		resp.Meta = m
	}

	return root.Domains, resp, err
}

// Get individual domain. It requires a non-empty domain name.
func (s *DomainsServiceOp) Get(ctx context.Context, name string) (*Domain, *Response, error) {
	if len(name) < 1 {
		return nil, nil, NewArgError("name", "cannot be an empty string")
	}

	path := fmt.Sprintf("%s/%s", domainsBasePath, name)

	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	root := new(domainRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	return root.Domain, resp, err
}

// Create a new domain
func (s *DomainsServiceOp) Create(ctx context.Context, createRequest *DomainCreateRequest) (*Domain, *Response, error) {
	if createRequest == nil {
		return nil, nil, NewArgError("createRequest", "cannot be nil")
	}

	path := domainsBasePath

	req, err := s.client.NewRequest(ctx, http.MethodPost, path, createRequest)
	if err != nil {
		return nil, nil, err
	}

	root := new(domainRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.Domain, resp, err
}

// Delete domain
func (s *DomainsServiceOp) Delete(ctx context.Context, name string) (*Response, error) {
	if len(name) < 1 {
		return nil, NewArgError("name", "cannot be an empty string")
	}

	path := fmt.Sprintf("%s/%s", domainsBasePath, name)

	req, err := s.client.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(ctx, req, nil)

	return resp, err
}

// Converts a DomainRecord to a string.
func (d DomainRecord) String() string {
	return Stringify(d)
}

// Converts a DomainRecordEditRequest to a string.
func (d DomainRecordEditRequest) String() string {
	return Stringify(d)
}

// Records returns a slice of DomainRecord for a domain.
func (s *DomainsServiceOp) Records(ctx context.Context, domain string, opt *ListOptions) ([]DomainRecord, *Response, error) {
	if len(domain) < 1 {
		return nil, nil, NewArgError("domain", "cannot be an empty string")
	}

	path := fmt.Sprintf("%s/%s/records", domainsBasePath, domain)
	path, err := addOptions(path, opt)
	if err != nil {
		return nil, nil, err
	}

	return s.records(ctx, path)
}

// RecordsByType returns a slice of DomainRecord for a domain matched by record type.
func (s *DomainsServiceOp) RecordsByType(ctx context.Context, domain, ofType string, opt *ListOptions) ([]DomainRecord, *Response, error) {
	if len(domain) < 1 {
		return nil, nil, NewArgError("domain", "cannot be an empty string")
	}

	if len(ofType) < 1 {
		return nil, nil, NewArgError("type", "cannot be an empty string")
	}

	path := fmt.Sprintf("%s/%s/records?type=%s", domainsBasePath, domain, ofType)
	path, err := addOptions(path, opt)
	if err != nil {
		return nil, nil, err
	}

	return s.records(ctx, path)
}

// RecordsByName returns a slice of DomainRecord for a domain matched by record name.
func (s *DomainsServiceOp) RecordsByName(ctx context.Context, domain, name string, opt *ListOptions) ([]DomainRecord, *Response, error) {
	if len(domain) < 1 {
		return nil, nil, NewArgError("domain", "cannot be an empty string")
	}

	if len(name) < 1 {
		return nil, nil, NewArgError("name", "cannot be an empty string")
	}

	path := fmt.Sprintf("%s/%s/records?name=%s", domainsBasePath, domain, name)
	path, err := addOptions(path, opt)
	if err != nil {
		return nil, nil, err
	}

	return s.records(ctx, path)
}

// RecordsByTypeAndName returns a slice of DomainRecord for a domain matched by record type and name.
func (s *DomainsServiceOp) RecordsByTypeAndName(ctx context.Context, domain, ofType, name string, opt *ListOptions) ([]DomainRecord, *Response, error) {
	if len(domain) < 1 {
		return nil, nil, NewArgError("domain", "cannot be an empty string")
	}

	if len(ofType) < 1 {
		return nil, nil, NewArgError("type", "cannot be an empty string")
	}

	if len(name) < 1 {
		return nil, nil, NewArgError("name", "cannot be an empty string")
	}

	path := fmt.Sprintf("%s/%s/records?type=%s&name=%s", domainsBasePath, domain, ofType, name)
	path, err := addOptions(path, opt)
	if err != nil {
		return nil, nil, err
	}

	return s.records(ctx, path)
}

// Record returns the record id from a domain
func (s *DomainsServiceOp) Record(ctx context.Context, domain string, id int) (*DomainRecord, *Response, error) {
	if len(domain) < 1 {
		return nil, nil, NewArgError("domain", "cannot be an empty string")
	}

	if id < 1 {
		return nil, nil, NewArgError("id", "cannot be less than 1")
	}

	path := fmt.Sprintf("%s/%s/records/%d", domainsBasePath, domain, id)

	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	record := new(domainRecordRoot)
	resp, err := s.client.Do(ctx, req, record)
	if err != nil {
		return nil, resp, err
	}

	return record.DomainRecord, resp, err
}

// DeleteRecord deletes a record from a domain identified by id
func (s *DomainsServiceOp) DeleteRecord(ctx context.Context, domain string, id int) (*Response, error) {
	if len(domain) < 1 {
		return nil, NewArgError("domain", "cannot be an empty string")
	}

	if id < 1 {
		return nil, NewArgError("id", "cannot be less than 1")
	}

	path := fmt.Sprintf("%s/%s/records/%d", domainsBasePath, domain, id)

	req, err := s.client.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(ctx, req, nil)

	return resp, err
}

// EditRecord edits a record using a DomainRecordEditRequest
func (s *DomainsServiceOp) EditRecord(ctx context.Context,
	domain string,
	id int,
	editRequest *DomainRecordEditRequest,
) (*DomainRecord, *Response, error) {
	if len(domain) < 1 {
		return nil, nil, NewArgError("domain", "cannot be an empty string")
	}

	if id < 1 {
		return nil, nil, NewArgError("id", "cannot be less than 1")
	}

	if editRequest == nil {
		return nil, nil, NewArgError("editRequest", "cannot be nil")
	}

	path := fmt.Sprintf("%s/%s/records/%d", domainsBasePath, domain, id)

	req, err := s.client.NewRequest(ctx, http.MethodPut, path, editRequest)
	if err != nil {
		return nil, nil, err
	}

	root := new(domainRecordRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	return root.DomainRecord, resp, err
}

// CreateRecord creates a record using a DomainRecordEditRequest
func (s *DomainsServiceOp) CreateRecord(ctx context.Context,
	domain string,
	createRequest *DomainRecordEditRequest) (*DomainRecord, *Response, error) {
	if len(domain) < 1 {
		return nil, nil, NewArgError("domain", "cannot be empty string")
	}

	if createRequest == nil {
		return nil, nil, NewArgError("createRequest", "cannot be nil")
	}

	path := fmt.Sprintf("%s/%s/records", domainsBasePath, domain)
	req, err := s.client.NewRequest(ctx, http.MethodPost, path, createRequest)

	if err != nil {
		return nil, nil, err
	}

	d := new(domainRecordRoot)
	resp, err := s.client.Do(ctx, req, d)
	if err != nil {
		return nil, resp, err
	}

	return d.DomainRecord, resp, err
}

// Performs a domain records request given a path.
func (s *DomainsServiceOp) records(ctx context.Context, path string) ([]DomainRecord, *Response, error) {
	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	root := new(domainRecordsRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	if l := root.Links; l != nil {
		resp.Links = l
	}

	return root.DomainRecords, resp, err
}
