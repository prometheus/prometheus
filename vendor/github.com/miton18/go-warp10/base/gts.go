package base

// GTS is the Warp10 representation of a GeoTimeSerie (GTS)
type GTS struct {
	// Name of the time serie
	ClassName string `json:"c"`
	// Key/value of the GTS labels (changing one key or his value create a new GTS)
	Labels Labels `json:"l"`
	// Key/value of the GTS attributes (can be setted/updated/removed without creating a new GTS)
	Attributes Attributes `json:"a"`
	// Timestamp of the last datapoint received on this GTS (% last activity window)
	LastActivity int64 `json:"la"`
	// Array of datapoints of this GTS
	Values Datapoints `json:"v"`
}

// GTSList is an array of GTS
type GTSList []*GTS

// Selector is a Warp10 selector
type Selector string

// Labels is a string key/value definition
type Labels map[string]string

// Attributes is a string key/value definition
type Attributes map[string]string

// Datapoints is the sensision datapoints representation
type Datapoints [][]interface{}
