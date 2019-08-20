package kms

import "testing"

func TestClient_DescribeRegions(t *testing.T) {

	response, err := debugClient.DescribeRegions()
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		t.Logf("Reuslt = %++v", response)
	}
}
