package schema

import "time"

// Network defines the schema of a network.
type Network struct {
	ID         int               `json:"id"`
	Name       string            `json:"name"`
	Created    time.Time         `json:"created"`
	IPRange    string            `json:"ip_range"`
	Subnets    []NetworkSubnet   `json:"subnets"`
	Routes     []NetworkRoute    `json:"routes"`
	Servers    []int             `json:"servers"`
	Protection NetworkProtection `json:"protection"`
	Labels     map[string]string `json:"labels"`
}

// NetworkSubnet represents a subnet of a network.
type NetworkSubnet struct {
	Type        string `json:"type"`
	IPRange     string `json:"ip_range"`
	NetworkZone string `json:"network_zone"`
	Gateway     string `json:"gateway,omitempty"`
	VSwitchID   int    `json:"vswitch_id,omitempty"`
}

// NetworkRoute represents a route of a network.
type NetworkRoute struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
}

// NetworkProtection represents the protection level of a network.
type NetworkProtection struct {
	Delete bool `json:"delete"`
}

// NetworkUpdateRequest defines the schema of the request to update a network.
type NetworkUpdateRequest struct {
	Name   string             `json:"name,omitempty"`
	Labels *map[string]string `json:"labels,omitempty"`
}

// NetworkUpdateResponse defines the schema of the response when updating a network.
type NetworkUpdateResponse struct {
	Network Network `json:"network"`
}

// NetworkListResponse defines the schema of the response when
// listing networks.
type NetworkListResponse struct {
	Networks []Network `json:"networks"`
}

// NetworkGetResponse defines the schema of the response when
// retrieving a single network.
type NetworkGetResponse struct {
	Network Network `json:"network"`
}

// NetworkCreateRequest defines the schema of the request to create a network.
type NetworkCreateRequest struct {
	Name    string             `json:"name"`
	IPRange string             `json:"ip_range"`
	Subnets []NetworkSubnet    `json:"subnets,omitempty"`
	Routes  []NetworkRoute     `json:"routes,omitempty"`
	Labels  *map[string]string `json:"labels,omitempty"`
}

// NetworkCreateResponse defines the schema of the response when
// creating a network.
type NetworkCreateResponse struct {
	Network Network `json:"network"`
}

// NetworkActionChangeIPRangeRequest defines the schema of the request to
// change the IP range of a network.
type NetworkActionChangeIPRangeRequest struct {
	IPRange string `json:"ip_range"`
}

// NetworkActionChangeIPRangeResponse defines the schema of the response when
// changing the IP range of a network.
type NetworkActionChangeIPRangeResponse struct {
	Action Action `json:"action"`
}

// NetworkActionAddSubnetRequest defines the schema of the request to
// add a subnet to a network.
type NetworkActionAddSubnetRequest struct {
	Type        string `json:"type"`
	IPRange     string `json:"ip_range,omitempty"`
	NetworkZone string `json:"network_zone"`
	Gateway     string `json:"gateway"`
	VSwitchID   int    `json:"vswitch_id,omitempty"`
}

// NetworkActionAddSubnetResponse defines the schema of the response when
// adding a subnet to a network.
type NetworkActionAddSubnetResponse struct {
	Action Action `json:"action"`
}

// NetworkActionDeleteSubnetRequest defines the schema of the request to
// delete a subnet from a network.
type NetworkActionDeleteSubnetRequest struct {
	IPRange string `json:"ip_range"`
}

// NetworkActionDeleteSubnetResponse defines the schema of the response when
// deleting a subnet from a network.
type NetworkActionDeleteSubnetResponse struct {
	Action Action `json:"action"`
}

// NetworkActionAddRouteRequest defines the schema of the request to
// add a route to a network.
type NetworkActionAddRouteRequest struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
}

// NetworkActionAddRouteResponse defines the schema of the response when
// adding a route to a network.
type NetworkActionAddRouteResponse struct {
	Action Action `json:"action"`
}

// NetworkActionDeleteRouteRequest defines the schema of the request to
// delete a route from a network.
type NetworkActionDeleteRouteRequest struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
}

// NetworkActionDeleteRouteResponse defines the schema of the response when
// deleting a route from a network.
type NetworkActionDeleteRouteResponse struct {
	Action Action `json:"action"`
}

// NetworkActionChangeProtectionRequest defines the schema of the request to
// change the resource protection of a network.
type NetworkActionChangeProtectionRequest struct {
	Delete *bool `json:"delete,omitempty"`
}

// NetworkActionChangeProtectionResponse defines the schema of the response when
// changing the resource protection of a network.
type NetworkActionChangeProtectionResponse struct {
	Action Action `json:"action"`
}
