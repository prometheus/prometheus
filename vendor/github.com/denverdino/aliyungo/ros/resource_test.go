package ros

import (
	"os"
	"testing"
)

func TestClient_DescribeResources(t *testing.T) {
	stackName := os.Getenv("StackName")
	stackId := os.Getenv("StackId")

	response, err := debugClientForTestCase.DescribeResources(stackId, stackName)
	if err != nil {
		t.Fatalf("Failed to DescribeResources %++v", err)
	} else {
		for index, item := range response {
			t.Logf("item[%d] =  %++v", index, item)
		}
	}
}

func TestClient_DescribeResource(t *testing.T) {
	stackName := os.Getenv("StackName")
	stackId := os.Getenv("StackId")
	resourceName := os.Getenv("ResourceName")

	response, err := debugClientForTestCase.DescribeResource(stackId, stackName, resourceName)
	if err != nil {
		t.Fatalf("Failed to DescribeResource %++v", err)
	} else {
		t.Logf("Resource = %++v", response)
	}
}

func TestClient_DescribeResoureTypes(t *testing.T) {

	response, err := debugClientForTestCase.DescribeResoureTypes("")
	if err != nil {
		t.Fatalf("Failed to DescribeResoureTypes %++v", err)
	} else {
		t.Logf("Resource = %++v", response)
	}
}

func TestClient_DescribeResoureType(t *testing.T) {
	resouceType := os.Getenv("ResourceType")
	response, err := debugClientForTestCase.DescribeResoureType(resouceType)
	if err != nil {
		t.Fatalf("Failed to DescribeResoureType %++v", err)
	} else {
		t.Logf("Resource = %++v", response)
	}
}

func TestClient_DescribeResoureTypeTemplate(t *testing.T) {
	resouceType := os.Getenv("ResourceType")
	response, err := debugClientForTestCase.DescribeResoureTypeTemplate(resouceType)
	if err != nil {
		t.Fatalf("Failed to DescribeResoureTypeTemplate %++v", err)
	} else {
		t.Logf("Resource = %++v", response)
	}
}
