package ecs

import (
	"testing"
)

var TestTags = map[string]string{
	"test":  "api",
	"gosdk": "1.0",
}

func TestAddTags(t *testing.T) {
	client := NewTestClient()

	instance, err := client.DescribeInstanceAttribute(TestInstanceId)
	if err != nil {
		t.Fatalf("Failed to DescribeInstanceAttribute for instance %s: %v", TestInstanceId, err)
	}

	args := AddTagsArgs{
		RegionId:     instance.RegionId,
		ResourceId:   instance.InstanceId,
		ResourceType: TagResourceInstance,
		Tag:          TestTags,
	}
	err = client.AddTags(&args)

	if err != nil {
		t.Errorf("Failed to AddTags for instance %s: %v", TestInstanceId, err)
	}

}

func TestDescribeResourceByTags(t *testing.T) {
	client := NewTestClient()

	instance, err := client.DescribeInstanceAttribute(TestInstanceId)
	if err != nil {
		t.Fatalf("Failed to DescribeInstanceAttribute for instance %s: %v", TestInstanceId, err)
	}

	args := DescribeResourceByTagsArgs{
		RegionId:     instance.RegionId,
		ResourceType: TagResourceInstance,
		Tag:          TestTags,
	}
	result, _, err := client.DescribeResourceByTags(&args)

	if err != nil {
		t.Errorf("Failed to DescribeResourceByTags: %v", err)
	} else {
		t.Logf("result: %v", result)
	}
}

func TestDescribeTags(t *testing.T) {
	client := NewTestClient()

	instance, err := client.DescribeInstanceAttribute(TestInstanceId)
	if err != nil {
		t.Fatalf("Failed to DescribeInstanceAttribute for instance %s: %v", TestInstanceId, err)
	}

	args := DescribeTagsArgs{
		RegionId:     instance.RegionId,
		ResourceType: TagResourceInstance,
	}
	result, _, err := client.DescribeTags(&args)

	if err != nil {
		t.Errorf("Failed to DescribeTags: %v", err)
	} else {
		t.Logf("result: %v", result)
	}
}

func TestRemoveTags(t *testing.T) {

	client := NewTestClient()

	instance, err := client.DescribeInstanceAttribute(TestInstanceId)
	if err != nil {
		t.Fatalf("Failed to DescribeInstanceAttribute for instance %s: %v", TestInstanceId, err)
	}

	args := RemoveTagsArgs{
		RegionId:     instance.RegionId,
		ResourceId:   instance.InstanceId,
		ResourceType: TagResourceInstance,
		Tag:          TestTags,
	}
	err = client.RemoveTags(&args)

	if err != nil {
		t.Errorf("Failed to RemoveTags for instance %s: %v", TestInstanceId, err)
	}

}
