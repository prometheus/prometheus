package collins

import (
	"github.com/schallert/iso8601"
)

// AssetFindOpts contains the various options for find queries to collins.
// It can be used to query for things like asset type, asset status or arbitrary
// attributes set on assets. It also provides options for pagination.
type AssetFindOpts struct {
	Details      bool   `url:"details,omitempty"`      // True to return full assets, false to return partials. Defaults to false.
	RemoteLookup bool   `url:"remoteLookup,omitempty"` // True to search remote datacenters as well. See the MultiCollins documentation.
	Operation    string `url:"operation,omitempty"`    // "AND" or "OR", defaults to "OR".
	Type         string `url:"type,omitempty"`         // A valid asset type (e.g. SERVER_NODE)
	Status       string `url:"status,omitempty"`       // A valid asset status (e.g. Unallocated)
	State        string `url:"state,omitempty"`        // A valid asset state (e.g. RUNNING)
	Attribute    string `url:"attribute,omitempty"`    // Specified as tagname;tagvalue. tagname can be a reserved tag such as CPU_COUNT, MEMORY_SIZE_BYTES, etc. Leave tagvalue blank to find assets missing a particular attribute.
	Query        string `url:"query,omitempty"`        // Specify a CQL query

	// These need to be pointers otherwise marshalling them will result in
	// all time fields being sent as "0001-01-01 00:00:00 +0000 UTC"
	CreatedBefore *iso8601.Time `url:"createdBefore,omitempty"` // ISO8601 formatted
	CreatedAfter  *iso8601.Time `url:"createdAfter,omitempty"`  // ISO8601 formatted
	UpdatedBefore *iso8601.Time `url:"updatedBefore,omitempty"` // ISO8601 formatted
	UpdatedAfter  *iso8601.Time `url:"updatedAfter,omitempty"`  // ISO8601 formatted

	// Embed pagination options directly in the options
	PageOpts
}

// AssetFindSimilarOpts contains the options for find similar queries to Collins.
// It currently only wraps the pagination options struct.
type AssetFindSimilarOpts struct {
	PageOpts
}

// AssetService.Find searches for assets in collins. See AssetFindOpts for
// available options.
//
// http://tumblr.github.io/collins/api.html#api-asset-asset-find
func (a AssetService) Find(opts *AssetFindOpts) ([]Asset, *Response, error) {
	ustr, err := addOptions("api/assets", opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := a.client.NewRequest("GET", ustr)
	if err != nil {
		return nil, nil, err
	}

	var c struct {
		Assets []Asset `json:"Data"`
	}

	resp, err := a.client.Do(req, &c)
	if err != nil {
		return nil, resp, err
	}

	return c.Assets, resp, nil
}

// AssetService.FindSimilar queries collins for assets that are similar to the
// asset represented by the tag passed as a parameter. See AssetFindOpts for
// available options.
//
// http://tumblr.github.io/collins/api.html#api-asset-asset-find-similar
func (a AssetService) FindSimilar(tag string, opts *AssetFindSimilarOpts) ([]Asset, *Response, error) {
	ustr, err := addOptions("api/asset/"+tag+"/similar", opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := a.client.NewRequest("GET", ustr)
	if err != nil {
		return nil, nil, err
	}

	var c struct {
		Meta []Metadata `json:"Data"`
	}

	resp, err := a.client.Do(req, &c)
	if err != nil {
		return nil, resp, err
	}

	assets := make([]Asset, len(c.Meta), len(c.Meta))
	for i := range assets {
		assets[i].Metadata = c.Meta[i]
	}

	return assets, resp, nil
}
