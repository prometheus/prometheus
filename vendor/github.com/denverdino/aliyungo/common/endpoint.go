package common

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"
)

const (
	// LocationDefaultEndpoint is the default API endpoint of Location services
	locationDefaultEndpoint = "https://location.aliyuncs.com"
	locationAPIVersion      = "2015-06-12"
	HTTP_PROTOCOL           = "http"
	HTTPS_PROTOCOL          = "https"
)

var (
	endpoints = make(map[Region]map[string]string)

	SpecailEnpoints = map[Region]map[string]string{
		APNorthEast1: {
			"ecs": "https://ecs.ap-northeast-1.aliyuncs.com",
			"slb": "https://slb.ap-northeast-1.aliyuncs.com",
			"rds": "https://rds.ap-northeast-1.aliyuncs.com",
			"vpc": "https://vpc.ap-northeast-1.aliyuncs.com",
		},
		APSouthEast2: {
			"ecs": "https://ecs.ap-southeast-2.aliyuncs.com",
			"slb": "https://slb.ap-southeast-2.aliyuncs.com",
			"rds": "https://rds.ap-southeast-2.aliyuncs.com",
			"vpc": "https://vpc.ap-southeast-2.aliyuncs.com",
		},
		APSouthEast3: {
			"ecs": "https://ecs.ap-southeast-3.aliyuncs.com",
			"slb": "https://slb.ap-southeast-3.aliyuncs.com",
			"rds": "https://rds.ap-southeast-3.aliyuncs.com",
			"vpc": "https://vpc.ap-southeast-3.aliyuncs.com",
		},
		MEEast1: {
			"ecs": "https://ecs.me-east-1.aliyuncs.com",
			"slb": "https://slb.me-east-1.aliyuncs.com",
			"rds": "https://rds.me-east-1.aliyuncs.com",
			"vpc": "https://vpc.me-east-1.aliyuncs.com",
		},
		EUCentral1: {
			"ecs": "https://ecs.eu-central-1.aliyuncs.com",
			"slb": "https://slb.eu-central-1.aliyuncs.com",
			"rds": "https://rds.eu-central-1.aliyuncs.com",
			"vpc": "https://vpc.eu-central-1.aliyuncs.com",
		},
		EUWest1: {
			"ecs": "https://ecs.eu-west-1.aliyuncs.com",
			"slb": "https://slb.eu-west-1.aliyuncs.com",
			"rds": "https://rds.eu-west-1.aliyuncs.com",
			"vpc": "https://vpc.eu-west-1.aliyuncs.com",
		},
		Zhangjiakou: {
			"ecs": "https://ecs.cn-zhangjiakou.aliyuncs.com",
			"slb": "https://slb.cn-zhangjiakou.aliyuncs.com",
			"rds": "https://rds.cn-zhangjiakou.aliyuncs.com",
			"vpc": "https://vpc.cn-zhangjiakou.aliyuncs.com",
		},
		Huhehaote: {
			"ecs": "https://ecs.cn-huhehaote.aliyuncs.com",
			"slb": "https://slb.cn-huhehaote.aliyuncs.com",
			"rds": "https://rds.cn-huhehaote.aliyuncs.com",
			"vpc": "https://vpc.cn-huhehaote.aliyuncs.com",
		},
	}
)

//init endpoints from file
func init() {

}

type LocationClient struct {
	Client
}

func NewLocationClient(accessKeyId, accessKeySecret, securityToken string) *LocationClient {
	endpoint := os.Getenv("LOCATION_ENDPOINT")
	if endpoint == "" {
		endpoint = locationDefaultEndpoint
	}

	client := &LocationClient{}
	client.Init(endpoint, locationAPIVersion, accessKeyId, accessKeySecret)
	client.securityToken = securityToken
	return client
}

func NewLocationClientWithSecurityToken(accessKeyId, accessKeySecret, securityToken string) *LocationClient {
	endpoint := os.Getenv("LOCATION_ENDPOINT")
	if endpoint == "" {
		endpoint = locationDefaultEndpoint
	}

	client := &LocationClient{}
	client.WithEndpoint(endpoint).
		WithVersion(locationAPIVersion).
		WithAccessKeyId(accessKeyId).
		WithAccessKeySecret(accessKeySecret).
		WithSecurityToken(securityToken).
		InitClient()
	return client
}

func (client *LocationClient) DescribeEndpoint(args *DescribeEndpointArgs) (*DescribeEndpointResponse, error) {
	response := &DescribeEndpointResponse{}
	err := client.Invoke("DescribeEndpoint", args, response)
	if err != nil {
		return nil, err
	}
	return response, err
}

func (client *LocationClient) DescribeEndpoints(args *DescribeEndpointsArgs) (*DescribeEndpointsResponse, error) {
	response := &DescribeEndpointsResponse{}
	err := client.Invoke("DescribeEndpoints", args, response)
	if err != nil {
		return nil, err
	}
	return response, err
}

func getProductRegionEndpoint(region Region, serviceCode string) string {
	if sp, ok := endpoints[region]; ok {
		if endpoint, ok := sp[serviceCode]; ok {
			return endpoint
		}
	}

	return ""
}

func setProductRegionEndpoint(region Region, serviceCode string, endpoint string) {
	endpoints[region] = map[string]string{
		serviceCode: endpoint,
	}
}

func (client *LocationClient) DescribeOpenAPIEndpoint(region Region, serviceCode string) string {
	if endpoint := getProductRegionEndpoint(region, serviceCode); endpoint != "" {
		return endpoint
	}
	defaultProtocols := HTTP_PROTOCOL

	args := &DescribeEndpointsArgs{
		Id:          region,
		ServiceCode: serviceCode,
		Type:        "openAPI",
	}

	var endpoint *DescribeEndpointsResponse
	var err error
	for index := 0; index < 5; index++ {
		endpoint, err = client.DescribeEndpoints(args)
		if err == nil && endpoint != nil && len(endpoint.Endpoints.Endpoint) > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if err != nil || endpoint == nil || len(endpoint.Endpoints.Endpoint) <= 0 {
		log.Printf("aliyungo: can not get endpoint from service, use default. endpoint=[%v], error=[%v]\n", endpoint, err)
		return ""
	}

	for _, protocol := range endpoint.Endpoints.Endpoint[0].Protocols.Protocols {
		if strings.ToLower(protocol) == HTTPS_PROTOCOL {
			defaultProtocols = HTTPS_PROTOCOL
			break
		}
	}

	ep := fmt.Sprintf("%s://%s", defaultProtocols, endpoint.Endpoints.Endpoint[0].Endpoint)

	setProductRegionEndpoint(region, serviceCode, ep)
	return ep
}

func loadEndpointFromFile(region Region, serviceCode string) string {
	data, err := ioutil.ReadFile("./endpoints.xml")
	if err != nil {
		return ""
	}
	var endpoints Endpoints
	err = xml.Unmarshal(data, &endpoints)
	if err != nil {
		return ""
	}
	for _, endpoint := range endpoints.Endpoint {
		if endpoint.RegionIds.RegionId == string(region) {
			for _, product := range endpoint.Products.Product {
				if strings.ToLower(product.ProductName) == serviceCode {
					return fmt.Sprintf("%s://%s", HTTPS_PROTOCOL, product.DomainName)
				}
			}
		}
	}

	return ""
}
