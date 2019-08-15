package ecs

// EndpointMap Endpoint Data
var EndpointMap map[string]string

// EndpointType regional or central
var EndpointType = "regional"

// GetEndpointMap Get Endpoint Data Map
func GetEndpointMap() map[string]string {
	if EndpointMap == nil {
		EndpointMap = map[string]string{
			"cn-shenzhen":    "ecs-cn-hangzhou.aliyuncs.com",
			"cn-qingdao":     "ecs-cn-hangzhou.aliyuncs.com",
			"cn-beijing":     "ecs-cn-hangzhou.aliyuncs.com",
			"cn-shanghai":    "ecs-cn-hangzhou.aliyuncs.com",
			"cn-hongkong":    "ecs-cn-hangzhou.aliyuncs.com",
			"ap-southeast-1": "ecs-cn-hangzhou.aliyuncs.com",
			"us-east-1":      "ecs-cn-hangzhou.aliyuncs.com",
			"us-west-1":      "ecs-cn-hangzhou.aliyuncs.com",
			"cn-hangzhou":    "ecs-cn-hangzhou.aliyuncs.com",
		}
	}
	return EndpointMap
}

// GetEndpointType Get Endpoint Type Value
func GetEndpointType() string {
	return EndpointType
}
