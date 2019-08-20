package ros

import (
	"testing"

	"fmt"
	"os"
	"time"
)

var (
	myTestTemplate = `
{
  "ROSTemplateFormatVersion": "2015-09-01",
  "Resources": {
    "string3": {
      "Type": "ALIYUN::RandomString",
      "DependsOn": [
        "string2"
      ],
      "Properties": {
        "sequence": "octdigits",
        "length": 100
      }
    },
    "string1": {
      "Test1": "Hello",
      "Type": "ALIYUN::RandomString"
    },
    "string2": {
      "Type": "ALIYUN::RandomString",
      "DependsOn": [
        "string1"
      ],
      "Properties": {
        "sequence": "octdigits",
        "length": 100
      }
    }
  }
}
	`

	myTestTemplateUpdate = `
{
  "ROSTemplateFormatVersion": "2015-09-01",
  "Resources": {
    "string3": {
      "Type": "ALIYUN::RandomString",
      "DependsOn": [
        "string2"
      ],
      "Properties": {
        "sequence": "octdigits",
        "length": 125
      }
    },
    "string1": {
      "Test1": "Hello222",
      "Type": "ALIYUN::RandomString"
    },
    "string2": {
      "Type": "ALIYUN::RandomString",
      "DependsOn": [
        "string1"
      ],
      "Properties": {
        "sequence": "octdigits",
        "length": 100
      }
    }
  }
}
	`
)

func TestClient_CreateStack(t *testing.T) {
	args := &CreateStackRequest{
		Name:            fmt.Sprintf("my-k8s-test-stack-%d", time.Now().Unix()),
		Template:        myTestTemplate,
		Parameters:      map[string]interface{}{},
		DisableRollback: false,
		TimeoutMins:     30,
	}

	response, err := debugClientForTestCase.CreateStack(TestRegionID, args)
	if err != nil {
		t.Fatalf("Failed to CreateStack %++v", err)
	} else {
		t.Logf("Success %++v", response)
	}
}

func TestClient_DeleteStack(t *testing.T) {
	stackName := os.Getenv("StackName")
	stackId := os.Getenv("StackId")

	response, err := debugClientForTestCase.DeleteStack(TestRegionID, stackId, stackName)
	if err != nil {
		t.Fatalf("Failed to DeleteStack %++v", err)
	} else {
		t.Logf("Success %++v", response)
	}
}

func TestClient_AbandonStack(t *testing.T) {
	stackName := os.Getenv("StackName")
	stackId := os.Getenv("StackId")

	response, err := debugClientForTestCase.AbandonStack(TestRegionID, stackId, stackName)
	if err != nil {
		t.Fatalf("Failed to AbandonStack %++v", err)
	} else {
		t.Logf("Success %++v", response)
	}
}

func TestClient_DescribeStacks(t *testing.T) {
	args := &DescribeStacksRequest{
		RegionId: TestRegionID,
	}

	stacks, err := debugClientForTestCase.DescribeStacks(args)
	if err != nil {
		t.Fatalf("Failed to DescribeStacks %++v", err)
	} else {
		t.Logf("Response is %++v", stacks)
	}
}

func TestClient_DescribeStack(t *testing.T) {
	stackName := os.Getenv("StackName")
	stackId := os.Getenv("StackId")

	response, err := debugClientForTestCase.DescribeStack(TestRegionID, stackId, stackName)
	if err != nil {
		t.Fatalf("Failed to DescribeStack %++v", err)
	} else {
		t.Logf("Success %++v", response)
	}
}

func TestClient_UpdateStack(t *testing.T) {
	stackName := os.Getenv("StackName")
	stackId := os.Getenv("StackId")

	//ps, _ := json.Marshal(p)
	args := &UpdateStackRequest{
		Template:        myTestTemplateUpdate,
		Parameters:      map[string]interface{}{},
		DisableRollback: false,
		TimeoutMins:     45,
	}

	response, err := debugClientForTestCase.UpdateStack(TestRegionID, stackId, stackName, args)
	if err != nil {
		t.Fatalf("Failed to UpdateStack %++v", err)
	} else {
		t.Logf("Success %++v", response)
	}
}
