package ros

import (
	"os"
	"testing"
)

func TestClient_DescribeTemplate(t *testing.T) {
	stackName := os.Getenv("StackName")
	stackId := os.Getenv("StackId")

	response, err := debugClientForTestCase.DescribeTemplate(stackId, stackName)
	if err != nil {
		t.Fatalf("Failed to DescribeTemplate %++v", err)
	} else {
		t.Logf("Resource = %++v", response.Mappings)
	}
}

func TestClient_ValidateTemplate(t *testing.T) {
	args := &ValidateTemplateRequest{
		Template: myTestTemplate,
	}
	response, err := debugClientForTestCase.ValidateTemplate(args)
	if err != nil {
		t.Fatalf("Failed to ValidateTemplate %++v", err)
	} else {
		t.Logf("Result = %++v", response)
	}
}
