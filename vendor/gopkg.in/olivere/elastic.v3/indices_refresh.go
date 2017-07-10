// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"fmt"
	"net/url"
	"strings"

	"gopkg.in/olivere/elastic.v3/uritemplates"
)

type RefreshService struct {
	client  *Client
	indices []string
	force   *bool
	pretty  bool
}

func NewRefreshService(client *Client) *RefreshService {
	builder := &RefreshService{
		client:  client,
		indices: make([]string, 0),
	}
	return builder
}

func (s *RefreshService) Index(indices ...string) *RefreshService {
	s.indices = append(s.indices, indices...)
	return s
}

func (s *RefreshService) Force(force bool) *RefreshService {
	s.force = &force
	return s
}

func (s *RefreshService) Pretty(pretty bool) *RefreshService {
	s.pretty = pretty
	return s
}

func (s *RefreshService) Do() (*RefreshResult, error) {
	// Build url
	path := "/"

	// Indices part
	indexPart := make([]string, 0)
	for _, index := range s.indices {
		index, err := uritemplates.Expand("{index}", map[string]string{
			"index": index,
		})
		if err != nil {
			return nil, err
		}
		indexPart = append(indexPart, index)
	}
	if len(indexPart) > 0 {
		path += strings.Join(indexPart, ",")
	}

	path += "/_refresh"

	// Parameters
	params := make(url.Values)
	if s.force != nil {
		params.Set("force", fmt.Sprintf("%v", *s.force))
	}
	if s.pretty {
		params.Set("pretty", fmt.Sprintf("%v", s.pretty))
	}

	// Get response
	res, err := s.client.PerformRequest("POST", path, params, nil)
	if err != nil {
		return nil, err
	}

	// Return result
	ret := new(RefreshResult)
	if err := s.client.decoder.Decode(res.Body, ret); err != nil {
		return nil, err
	}
	return ret, nil
}

// -- Result of a refresh request.

type RefreshResult struct {
	Shards shardsInfo `json:"_shards,omitempty"`
}
