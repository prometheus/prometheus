package ecs

import (
	"os"
	"testing"
	"time"
)

func TestDisks(t *testing.T) {

	client := NewTestClient()

	instance, err := client.DescribeInstanceAttribute(TestInstanceId)
	if err != nil {
		t.Fatalf("Failed to DescribeInstanceAttribute for instance %s: %v", TestInstanceId, err)
	}

	args := DescribeDisksArgs{}

	args.InstanceId = TestInstanceId
	args.RegionId = instance.RegionId
	disks, _, err := client.DescribeDisks(&args)

	if err != nil {
		t.Fatalf("Failed to DescribeDisks for instance %s: %v", TestInstanceId, err)
	}

	for _, disk := range disks {
		t.Logf("Disk of instance %s: %++v", TestInstanceId, disk)
	}
}

func TestDiskCreationAndDeletion(t *testing.T) {

	if TestIAmRich == false { //Avoid payment
		return
	}

	client := NewTestClient()

	instance, err := client.DescribeInstanceAttribute(TestInstanceId)
	if err != nil {
		t.Fatalf("Failed to DescribeInstanceAttribute for instance %s: %v", TestInstanceId, err)
	}

	args := CreateDiskArgs{
		RegionId: instance.RegionId,
		ZoneId:   instance.ZoneId,
		DiskName: "test-disk",
		Size:     5,
	}

	diskId, err := client.CreateDisk(&args)
	if err != nil {
		t.Fatalf("Failed to create disk: %v", err)
	}
	t.Logf("Create disk %s successfully", diskId)

	attachArgs := AttachDiskArgs{
		InstanceId: instance.InstanceId,
		DiskId:     diskId,
	}

	err = client.AttachDisk(&attachArgs)
	if err != nil {
		t.Errorf("Failed to create disk: %v", err)
	} else {
		t.Logf("Attach disk %s to instance %s successfully", diskId, instance.InstanceId)

		instance, err = client.DescribeInstanceAttribute(TestInstanceId)
		if err != nil {
			t.Errorf("Failed to DescribeInstanceAttribute for instance %s: %v", TestInstanceId, err)
		} else {
			t.Logf("Instance: %++v  %v", instance, err)
		}
		err = client.WaitForDisk(instance.RegionId, diskId, DiskStatusInUse, 0)
		if err != nil {
			t.Fatalf("Failed to wait for disk %s to status %s: %v", diskId, DiskStatusInUse, err)
		}
		err = client.DetachDisk(instance.InstanceId, diskId)
		if err != nil {
			t.Errorf("Failed to detach disk: %v", err)
		} else {
			t.Logf("Detach disk %s to instance %s successfully", diskId, instance.InstanceId)
		}

		err = client.WaitForDisk(instance.RegionId, diskId, DiskStatusAvailable, 0)
		if err != nil {
			t.Fatalf("Failed to wait for disk %s to status %s: %v", diskId, DiskStatusAvailable, err)
		}
	}
	err = client.DeleteDisk(diskId)
	if err != nil {
		t.Fatalf("Failed to delete disk %s: %v", diskId, err)
	}
	t.Logf("Delete disk %s successfully", diskId)
}

func TestReplaceSystemDiskUsingSizeParam(t *testing.T) {
	client := NewTestClientForDebug()

	args := ReplaceSystemDiskArgs{
		InstanceId: TestInstanceId,
		ImageId:    TestImageId,
		SystemDisk: SystemDiskType{
			Size: 192,
		},
		ClientToken: client.GenerateClientToken(),
	}

	diskId, err := client.ReplaceSystemDisk(&args)
	if err != nil {
		t.Errorf("Failed to replace system disk %v", err)
	} else {
		t.Logf("diskId is %s", diskId)
	}
}

func TestReplaceSystemDisk(t *testing.T) {
	client := NewTestClient()

	err := client.WaitForInstance(TestInstanceId, Running, 0)
	err = client.StopInstance(TestInstanceId, true)
	if err != nil {
		t.Errorf("Failed to stop instance %s: %v", TestInstanceId, err)
	}
	err = client.WaitForInstance(TestInstanceId, Stopped, 0)
	if err != nil {
		t.Errorf("Instance %s is failed to stop: %v", TestInstanceId, err)
	}
	t.Logf("Instance %s is stopped successfully.", TestInstanceId)

	args := ReplaceSystemDiskArgs{
		InstanceId: TestInstanceId,
		ImageId:    TestImageId,
	}

	diskId, err := client.ReplaceSystemDisk(&args)
	if err != nil {
		t.Errorf("Failed to replace system disk %v", err)
	}
	err = client.WaitForInstance(TestInstanceId, Stopped, 60)
	err = client.StartInstance(TestInstanceId)
	if err != nil {
		t.Errorf("Failed to start instance %s: %v", TestInstanceId, err)
	} else {
		err = client.WaitForInstance(TestInstanceId, Running, 0)
		if err != nil {
			t.Errorf("Failed to wait for instance %s running: %v", TestInstanceId, err)
		}
	}
	t.Logf("Replace system disk %s successfully ", diskId)
}

func TestResizeDisk(t *testing.T) {
	accessKeyId := os.Getenv("ACCESS_KEY_ID")
	accessKeySecret := os.Getenv("ACCESS_KEY_SECRET")

	client := NewClient(accessKeyId, accessKeySecret)

	args := CreateDiskArgs{
		RegionId:     "cn-beijing",
		ZoneId:       "cn-beijing-a",
		Size:         40,
		DiskCategory: DiskCategoryCloudEfficiency,
	}
	diskId, err := client.CreateDisk(&args)

	if err != nil {
		t.Errorf("CreateDisk failed %v", err)
	}

	time.Sleep(time.Duration(90) * time.Second) // wait for disk inner status ready to resize
	err = client.ResizeDisk(diskId, 60)

	if err != nil {
		t.Errorf("ResizeDisk failed %v", err)
	}

	_ = client.DeleteDisk(diskId)
}
