package godo

import (
	"context"
	"net/http"
	"time"
)

const billingHistoryBasePath = "v2/customers/my/billing_history"

// BillingHistoryService is an interface for interfacing with the BillingHistory
// endpoints of the DigitalOcean API
// See: https://developers.digitalocean.com/documentation/v2/#billing_history
type BillingHistoryService interface {
	List(context.Context, *ListOptions) (*BillingHistory, *Response, error)
}

// BillingHistoryServiceOp handles communication with the BillingHistory related methods of
// the DigitalOcean API.
type BillingHistoryServiceOp struct {
	client *Client
}

var _ BillingHistoryService = &BillingHistoryServiceOp{}

// BillingHistory represents a DigitalOcean Billing History
type BillingHistory struct {
	BillingHistory []BillingHistoryEntry `json:"billing_history"`
	Links          *Links                `json:"links"`
	Meta           *Meta                 `json:"meta"`
}

// BillingHistoryEntry represents an entry in a customer's Billing History
type BillingHistoryEntry struct {
	Description string    `json:"description"`
	Amount      string    `json:"amount"`
	InvoiceID   *string   `json:"invoice_id"`
	InvoiceUUID *string   `json:"invoice_uuid"`
	Date        time.Time `json:"date"`
	Type        string    `json:"type"`
}

func (b BillingHistory) String() string {
	return Stringify(b)
}

// List the Billing History for a customer
func (s *BillingHistoryServiceOp) List(ctx context.Context, opt *ListOptions) (*BillingHistory, *Response, error) {
	path, err := addOptions(billingHistoryBasePath, opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	root := new(BillingHistory)
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

	return root, resp, err
}
