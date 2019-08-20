package oss

import (
	"fmt"
)

// Region represents OSS region
type Region string

// Constants of region definition
const (
	Hangzhou    = Region("oss-cn-hangzhou")
	Qingdao     = Region("oss-cn-qingdao")
	Beijing     = Region("oss-cn-beijing")
	Hongkong    = Region("oss-cn-hongkong")
	Shenzhen    = Region("oss-cn-shenzhen")
	Shanghai    = Region("oss-cn-shanghai")
	Zhangjiakou = Region("oss-cn-zhangjiakou")
	Huhehaote   = Region("oss-cn-huhehaote")

	USWest1      = Region("oss-us-west-1")
	USEast1      = Region("oss-us-east-1")
	APSouthEast1 = Region("oss-ap-southeast-1")
	APNorthEast1 = Region("oss-ap-northeast-1")
	APSouthEast2 = Region("oss-ap-southeast-2")

	MEEast1 = Region("oss-me-east-1")

	EUCentral1 = Region("oss-eu-central-1")
	EUWest1    = Region("oss-eu-west-1")

	DefaultRegion = Hangzhou
)

// GetEndpoint returns endpoint of region
func (r Region) GetEndpoint(internal bool, bucket string, secure bool) string {
	if internal {
		return r.GetInternalEndpoint(bucket, secure)
	}
	return r.GetInternetEndpoint(bucket, secure)
}

func getProtocol(secure bool) string {
	protocol := "http"
	if secure {
		protocol = "https"
	}
	return protocol
}

// GetInternetEndpoint returns internet endpoint of region
func (r Region) GetInternetEndpoint(bucket string, secure bool) string {
	protocol := getProtocol(secure)
	if bucket == "" {
		return fmt.Sprintf("%s://oss.aliyuncs.com", protocol)
	}
	return fmt.Sprintf("%s://%s.%s.aliyuncs.com", protocol, bucket, string(r))
}

// GetInternalEndpoint returns internal endpoint of region
func (r Region) GetInternalEndpoint(bucket string, secure bool) string {
	protocol := getProtocol(secure)
	if bucket == "" {
		return fmt.Sprintf("%s://oss-internal.aliyuncs.com", protocol)
	}
	return fmt.Sprintf("%s://%s.%s-internal.aliyuncs.com", protocol, bucket, string(r))
}

// GetInternalEndpoint returns internal endpoint of region
func (r Region) GetVPCInternalEndpoint(bucket string, secure bool) string {
	protocol := getProtocol(secure)
	if bucket == "" {
		return fmt.Sprintf("%s://vpc100-oss-cn-hangzhou.aliyuncs.com", protocol)
	}
	if r == USEast1 {
		return r.GetInternalEndpoint(bucket, secure)
	} else {
		return fmt.Sprintf("%s://%s.vpc100-%s.aliyuncs.com", protocol, bucket, string(r))
	}
}
