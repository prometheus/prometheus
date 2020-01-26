package apiversions

import (
	"github.com/gophercloud/gophercloud"
)

// APIVersions represents the result from getting a list of all versions available
type APIVersions struct {
	DefaultVersion APIVersion   `json:"default_version"`
	Versions       []APIVersion `json:"versions"`
}

// APIVersion represents an API version for Ironic
type APIVersion struct {
	// ID is the unique identifier of the API version.
	ID string `json:"id"`

	// MinVersion is the minimum microversion supported.
	MinVersion string `json:"min_version"`

	// Status is the API versions status.
	Status string `json:"status"`

	// Version is the maximum microversion supported.
	Version string `json:"version"`
}

// GetResult represents the result of a get operation.
type GetResult struct {
	gophercloud.Result
}

// ListResult represents the result of a list operation.
type ListResult struct {
	gophercloud.Result
}

// Extract is a function that accepts a get result and extracts an API version resource.
func (r GetResult) Extract() (*APIVersion, error) {
	var s struct {
		Version APIVersion `json:"version"`
	}

	err := r.ExtractInto(&s)
	if err != nil {
		return nil, err
	}

	return &s.Version, nil
}

// Extract is a function that accepts a list result and extracts an APIVersions resource
func (r ListResult) Extract() (*APIVersions, error) {
	var version APIVersions

	err := r.ExtractInto(&version)
	if err != nil {
		return nil, err
	}

	return &version, nil
}
