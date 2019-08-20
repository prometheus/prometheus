package ecs

import (
	"testing"

	"github.com/denverdino/aliyungo/common"
)

func TestImageCreationAndDeletion(t *testing.T) {

	client := NewTestClient()

	instance, err := client.DescribeInstanceAttribute(TestInstanceId)
	if err != nil {
		t.Fatalf("Failed to DescribeInstanceAttribute for instance %s: %v", TestInstanceId, err)
	}

	args := DescribeSnapshotsArgs{}

	args.InstanceId = TestInstanceId
	args.RegionId = instance.RegionId
	snapshots, _, err := client.DescribeSnapshots(&args)

	if err != nil {
		t.Errorf("Failed to DescribeSnapshots for instance %s: %v", TestInstanceId, err)
	}

	if len(snapshots) > 0 {

		createImageArgs := CreateImageArgs{
			RegionId:   instance.RegionId,
			SnapshotId: snapshots[0].SnapshotId,

			ImageName:    "My_Test_Image_for_AliyunGo",
			ImageVersion: "1.0",
			Description:  "My Test Image for AliyunGo description",
			ClientToken:  client.GenerateClientToken(),
		}
		imageId, err := client.CreateImage(&createImageArgs)
		if err != nil {
			t.Errorf("Failed to CreateImage for SnapshotId %s: %v", createImageArgs.SnapshotId, err)
		}
		t.Logf("Image %s is created successfully.", imageId)

		err = client.DeleteImage(instance.RegionId, imageId)
		if err != nil {
			t.Errorf("Failed to DeleteImage for %s: %v", imageId, err)
		}
		t.Logf("Image %s is deleted successfully.", imageId)

	}
}

func TestModifyImageSharePermission(t *testing.T) {
	req := ModifyImageSharePermissionArgs{
		RegionId:   common.Beijing,
		ImageId:    TestImageId,
		AddAccount: []string{TestAccountId},
	}
	client := NewTestClient()
	err := client.ModifyImageSharePermission(&req)
	if err != nil {
		t.Errorf("Failed to ShareImage: %v", err)
	}

	shareInfo, err := client.DescribeImageSharePermission(&req)
	if err != nil {
		t.Errorf("Failed to ShareImage: %v", err)
	}
	t.Logf("result:image: %++v", shareInfo)
}

func TestCopyImage(t *testing.T) {
	client := NewTestClient()
	req := CopyImageArgs{
		RegionId:               common.Beijing,
		ImageId:                TestImageId,
		DestinationRegionId:    common.Hangzhou,
		DestinationImageName:   "My_Test_Image_NAME_for_AliyunGo",
		DestinationDescription: "My Test Image for AliyunGo description",
		ClientToken:            client.GenerateClientToken(),
	}

	imageId, err := client.CopyImage(&req)
	if err != nil {
		t.Errorf("Failed to CopyImage: %v", err)
	}
	t.Logf("result:image: %++v", imageId)

	if err := client.WaitForImageReady(common.Hangzhou, imageId, 600); err != nil {
		t.Errorf("Failed to WaitImage: %v", err)
		//return
	}

	describeReq := DescribeImagesArgs{
		RegionId:        common.Hangzhou,
		ImageId:         imageId,
		Status:          ImageStatusAvailable,
		ImageOwnerAlias: ImageOwnerSelf,
	}

	images, _, err := client.DescribeImages(&describeReq)
	if err != nil {
		t.Errorf("Failed to describeImage: %v", err)
	}
	t.Logf("result: images %++v", images)
}

func TestCancelCopyImage(t *testing.T) {
	client := NewTestClient()
	if err := client.CancelCopyImage(common.Hangzhou, TestImageId); err != nil {
		t.Errorf("Failed to CancelCopyImage: %v", err)
	}
}
