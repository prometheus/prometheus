package common

type InternetChargeType string

const (
	PayByBandwidth = InternetChargeType("PayByBandwidth")
	PayByTraffic   = InternetChargeType("PayByTraffic")
)

type InstanceChargeType string

const (
	PrePaid  = InstanceChargeType("PrePaid")
	PostPaid = InstanceChargeType("PostPaid")
)

type DescribeEndpointArgs struct {
	Id          Region
	ServiceCode string
	Type        string
}

type EndpointItem struct {
	Protocols struct {
		Protocols []string
	}
	Type        string
	Namespace   string
	Id          Region
	SerivceCode string
	Endpoint    string
}

type DescribeEndpointResponse struct {
	Response
	EndpointItem
}

type DescribeEndpointsArgs struct {
	Id          Region
	ServiceCode string
	Type        string
}

type DescribeEndpointsResponse struct {
	Response
	Endpoints APIEndpoints
	RequestId string
	Success   bool
}

type APIEndpoints struct {
	Endpoint []EndpointItem
}

type NetType string

const (
	Internet = NetType("Internet")
	Intranet = NetType("Intranet")
)

type TimeType string

const (
	Hour  = TimeType("Hour")
	Day   = TimeType("Day")
	Week  = TimeType("Week")
	Month = TimeType("Month")
	Year  = TimeType("Year")
)

type NetworkType string

const (
	Classic = NetworkType("Classic")
	VPC     = NetworkType("VPC")
)

type BusinessInfo struct {
	Pack       string `json:"pack,omitempty"`
	ActivityId string `json:"activityId,omitempty"`
}

//xml
type Endpoints struct {
	Endpoint []Endpoint `xml:"Endpoint"`
}

type Endpoint struct {
	Name      string    `xml:"name,attr"`
	RegionIds RegionIds `xml:"RegionIds"`
	Products  Products  `xml:"Products"`
}

type RegionIds struct {
	RegionId string `xml:"RegionId"`
}

type Products struct {
	Product []Product `xml:"Product"`
}

type Product struct {
	ProductName string `xml:"ProductName"`
	DomainName  string `xml:"DomainName"`
}
