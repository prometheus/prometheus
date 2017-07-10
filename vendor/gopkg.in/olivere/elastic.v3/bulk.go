// Copyright 2012-present Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"

	"gopkg.in/olivere/elastic.v3/uritemplates"
)

// BulkService allows for batching bulk requests and sending them to
// Elasticsearch in one roundtrip. Use the Add method with BulkIndexRequest,
// BulkUpdateRequest, and BulkDeleteRequest to add bulk requests to a batch,
// then use Do to send them to Elasticsearch.
//
// BulkService will be reset after each Do call. In other words, you can
// reuse BulkService to send many batches. You do not have to create a new
// BulkService for each batch.
//
// See https://www.elastic.co/guide/en/elasticsearch/reference/2.x/docs-bulk.html
// for more details.
type BulkService struct {
	client *Client

	index    string
	typ      string
	requests []BulkableRequest
	timeout  string
	refresh  *bool
	pretty   bool

	sizeInBytes int64
}

// NewBulkService initializes a new BulkService.
func NewBulkService(client *Client) *BulkService {
	builder := &BulkService{
		client:   client,
		requests: make([]BulkableRequest, 0),
	}
	return builder
}

func (s *BulkService) reset() {
	s.requests = make([]BulkableRequest, 0)
	s.sizeInBytes = 0
}

// Index specifies the index to use for all batches. You may also leave
// this blank and specify the index in the individual bulk requests.
func (s *BulkService) Index(index string) *BulkService {
	s.index = index
	return s
}

// Type specifies the type to use for all batches. You may also leave
// this blank and specify the type in the individual bulk requests.
func (s *BulkService) Type(typ string) *BulkService {
	s.typ = typ
	return s
}

// Timeout is a global timeout for processing bulk requests. This is a
// server-side timeout, i.e. it tells Elasticsearch the time after which
// it should stop processing.
func (s *BulkService) Timeout(timeout string) *BulkService {
	s.timeout = timeout
	return s
}

// Refresh, when set to true, tells Elasticsearch to make the bulk requests
// available to search immediately after being processed. Normally, this
// only happens after a specified refresh interval.
func (s *BulkService) Refresh(refresh bool) *BulkService {
	s.refresh = &refresh
	return s
}

// Pretty tells Elasticsearch whether to return a formatted JSON response.
func (s *BulkService) Pretty(pretty bool) *BulkService {
	s.pretty = pretty
	return s
}

// Add adds bulkable requests, i.e. BulkIndexRequest, BulkUpdateRequest,
// and/or BulkDeleteRequest.
func (s *BulkService) Add(requests ...BulkableRequest) *BulkService {
	for _, r := range requests {
		s.requests = append(s.requests, r)
		s.sizeInBytes += s.estimateSizeInBytes(r)
	}
	return s
}

// EstimatedSizeInBytes returns the estimated size of all bulkable
// requests added via Add.
func (s *BulkService) EstimatedSizeInBytes() int64 {
	return s.sizeInBytes
}

// estimateSizeInBytes returns the estimates size of the given
// bulkable request, i.e. BulkIndexRequest, BulkUpdateRequest, and
// BulkDeleteRequest.
func (s *BulkService) estimateSizeInBytes(r BulkableRequest) int64 {
	lines, _ := r.Source()
	size := 0
	for _, line := range lines {
		// +1 for the \n
		size += len(line) + 1
	}
	return int64(size)
}

// NumberOfActions returns the number of bulkable requests that need to
// be sent to Elasticsearch on the next batch.
func (s *BulkService) NumberOfActions() int {
	return len(s.requests)
}

func (s *BulkService) bodyAsString() (string, error) {
	buf := bytes.NewBufferString("")

	for _, req := range s.requests {
		source, err := req.Source()
		if err != nil {
			return "", err
		}
		for _, line := range source {
			_, err := buf.WriteString(fmt.Sprintf("%s\n", line))
			if err != nil {
				return "", nil
			}
		}
	}

	return buf.String(), nil
}

// Do sends the batched requests to Elasticsearch. Note that, when successful,
// you can reuse the BulkService for the next batch as the list of bulk
// requests is cleared on success.
func (s *BulkService) Do() (*BulkResponse, error) {
	// No actions?
	if s.NumberOfActions() == 0 {
		return nil, errors.New("elastic: No bulk actions to commit")
	}

	// Get body
	body, err := s.bodyAsString()
	if err != nil {
		return nil, err
	}

	// Build url
	path := "/"
	if s.index != "" {
		index, err := uritemplates.Expand("{index}", map[string]string{
			"index": s.index,
		})
		if err != nil {
			return nil, err
		}
		path += index + "/"
	}
	if s.typ != "" {
		typ, err := uritemplates.Expand("{type}", map[string]string{
			"type": s.typ,
		})
		if err != nil {
			return nil, err
		}
		path += typ + "/"
	}
	path += "_bulk"

	// Parameters
	params := make(url.Values)
	if s.pretty {
		params.Set("pretty", fmt.Sprintf("%v", s.pretty))
	}
	if s.refresh != nil {
		params.Set("refresh", fmt.Sprintf("%v", *s.refresh))
	}
	if s.timeout != "" {
		params.Set("timeout", s.timeout)
	}

	// Get response
	res, err := s.client.PerformRequest("POST", path, params, body)
	if err != nil {
		return nil, err
	}

	// Return results
	ret := new(BulkResponse)
	if err := s.client.decoder.Decode(res.Body, ret); err != nil {
		return nil, err
	}

	// Reset so the request can be reused
	s.reset()

	return ret, nil
}

// BulkResponse is a response to a bulk execution.
//
// Example:
// {
//   "took":3,
//   "errors":false,
//   "items":[{
//     "index":{
//       "_index":"index1",
//       "_type":"tweet",
//       "_id":"1",
//       "_version":3,
//       "status":201
//     }
//   },{
//     "index":{
//       "_index":"index2",
//       "_type":"tweet",
//       "_id":"2",
//       "_version":3,
//       "status":200
//     }
//   },{
//     "delete":{
//       "_index":"index1",
//       "_type":"tweet",
//       "_id":"1",
//       "_version":4,
//       "status":200,
//       "found":true
//     }
//   },{
//     "update":{
//       "_index":"index2",
//       "_type":"tweet",
//       "_id":"2",
//       "_version":4,
//       "status":200
//     }
//   }]
// }
type BulkResponse struct {
	Took   int                            `json:"took,omitempty"`
	Errors bool                           `json:"errors,omitempty"`
	Items  []map[string]*BulkResponseItem `json:"items,omitempty"`
}

// BulkResponseItem is the result of a single bulk request.
type BulkResponseItem struct {
	Index   string        `json:"_index,omitempty"`
	Type    string        `json:"_type,omitempty"`
	Id      string        `json:"_id,omitempty"`
	Version int           `json:"_version,omitempty"`
	Status  int           `json:"status,omitempty"`
	Found   bool          `json:"found,omitempty"`
	Error   *ErrorDetails `json:"error,omitempty"`
}

// Indexed returns all bulk request results of "index" actions.
func (r *BulkResponse) Indexed() []*BulkResponseItem {
	return r.ByAction("index")
}

// Created returns all bulk request results of "create" actions.
func (r *BulkResponse) Created() []*BulkResponseItem {
	return r.ByAction("create")
}

// Updated returns all bulk request results of "update" actions.
func (r *BulkResponse) Updated() []*BulkResponseItem {
	return r.ByAction("update")
}

// Deleted returns all bulk request results of "delete" actions.
func (r *BulkResponse) Deleted() []*BulkResponseItem {
	return r.ByAction("delete")
}

// ByAction returns all bulk request results of a certain action,
// e.g. "index" or "delete".
func (r *BulkResponse) ByAction(action string) []*BulkResponseItem {
	if r.Items == nil {
		return nil
	}
	items := make([]*BulkResponseItem, 0)
	for _, item := range r.Items {
		if result, found := item[action]; found {
			items = append(items, result)
		}
	}
	return items
}

// ById returns all bulk request results of a given document id,
// regardless of the action ("index", "delete" etc.).
func (r *BulkResponse) ById(id string) []*BulkResponseItem {
	if r.Items == nil {
		return nil
	}
	items := make([]*BulkResponseItem, 0)
	for _, item := range r.Items {
		for _, result := range item {
			if result.Id == id {
				items = append(items, result)
			}
		}
	}
	return items
}

// Failed returns those items of a bulk response that have errors,
// i.e. those that don't have a status code between 200 and 299.
func (r *BulkResponse) Failed() []*BulkResponseItem {
	if r.Items == nil {
		return nil
	}
	errors := make([]*BulkResponseItem, 0)
	for _, item := range r.Items {
		for _, result := range item {
			if !(result.Status >= 200 && result.Status <= 299) {
				errors = append(errors, result)
			}
		}
	}
	return errors
}

// Succeeded returns those items of a bulk response that have no errors,
// i.e. those have a status code between 200 and 299.
func (r *BulkResponse) Succeeded() []*BulkResponseItem {
	if r.Items == nil {
		return nil
	}
	succeeded := make([]*BulkResponseItem, 0)
	for _, item := range r.Items {
		for _, result := range item {
			if result.Status >= 200 && result.Status <= 299 {
				succeeded = append(succeeded, result)
			}
		}
	}
	return succeeded
}
