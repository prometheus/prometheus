package schema

// Location defines the schema of a location.
type Location struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Country     string  `json:"country"`
	City        string  `json:"city"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	NetworkZone string  `json:"network_zone"`
}

// LocationGetResponse defines the schema of the response when retrieving a single location.
type LocationGetResponse struct {
	Location Location `json:"location"`
}

// LocationListResponse defines the schema of the response when listing locations.
type LocationListResponse struct {
	Locations []Location `json:"locations"`
}
