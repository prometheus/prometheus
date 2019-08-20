package slb

import (
	"encoding/json"
	"testing"

	"github.com/denverdino/aliyungo/common"
)

func TestAddTags(t *testing.T) {
	client := NewTestClientForDebug()

	tagItemArr := []TagItem{
		TagItem{TagKey: "username", TagValue: "test"},
		TagItem{TagKey: "birdthday", TagValue: "20170101"},
	}
	tagItems, _ := json.Marshal(tagItemArr)

	args := &AddTagsArgs{
		RegionId:       common.Beijing,
		LoadBalancerID: TestLoadBlancerID,
		Tags:           string(tagItems),
	}

	err := client.AddTags(args)
	if err != nil {
		t.Fatalf("Failed to add tags %++v", err)
	}

	t.Logf("Successfully to add tags")
}

func TestRemoveTags(t *testing.T) {
	client := NewTestClientForDebug()

	tagItemArr := []TagItem{
		TagItem{TagKey: "username", TagValue: "test"},
		TagItem{TagKey: "birdthday", TagValue: "20170101"},
	}
	tagItems, _ := json.Marshal(tagItemArr)

	args := &RemoveTagsArgs{
		RegionId:       common.Beijing,
		LoadBalancerID: TestLoadBlancerID,
		Tags:           string(tagItems),
	}

	err := client.RemoveTags(args)
	if err != nil {
		t.Fatalf("Failed to remove tags %++v", err)
	}

	t.Logf("Successfully to remove tags")
}

func TestDescribeTags(t *testing.T) {
	client := NewTestClientForDebug()

	args := &DescribeTagsArgs{
		RegionId:       common.Beijing,
		LoadBalancerID: TestLoadBlancerID,
	}

	tags, _, err := client.DescribeTags(args)
	if err != nil {
		t.Fatalf("Failed to describe tags %++v", err)
	}

	t.Logf("tags is %++v", tags)
}
