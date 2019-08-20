package common

import (
	"testing"
)

func TestLoadEndpointFromFile(t *testing.T) {

}

func TestClient_DescribeOpenAPIEndpoint(t *testing.T) {
	client := NewTestClientForDebug()
	services := []string{"ecs", "slb", "rds", "vpc"}
	regions := []Region{
		APNorthEast1,
		APSouthEast2,
		APSouthEast3,
		MEEast1,
		EUCentral1,
		Zhangjiakou,
		Huhehaote,
		Hangzhou,
		Qingdao,
		Beijing,
		Shanghai,
		Shenzhen,
	}

	for _, region := range regions {
		for _, service := range services {
			endpoint := client.DescribeOpenAPIEndpoint(Region(region), service)
			t.Logf("Endpoint[%s][%s]=%s", string(region), service, endpoint)
		}
	}
}

func TestLocationClient_DescribeEndpoints(t *testing.T) {
	client := NewTestClientForDebug()
	args := &DescribeEndpointsArgs{
		Id:          "cn-zhangjiakou",
		ServiceCode: "slb",
		Type:        "openAPI",
	}
	response, err := client.DescribeEndpoints(args)
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		t.Logf("Result = %++v", response)
	}
}
