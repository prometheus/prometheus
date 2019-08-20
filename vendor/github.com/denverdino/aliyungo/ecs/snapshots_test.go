package ecs

import (
	"testing"
)

func TestSnapshot(t *testing.T) {

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

	for _, snapshot := range snapshots {
		t.Logf("Snapshot of instance %s: %++v", TestInstanceId, snapshot)
	}
}

func TestSnapshotCreationAndDeletion(t *testing.T) {
	if TestQuick {
		return
	}

	client := NewTestClient()

	instance, err := client.DescribeInstanceAttribute(TestInstanceId)
	if err != nil {
		t.Fatalf("Failed to DescribeInstanceAttribute for instance %s: %v", TestInstanceId, err)
	}

	//Describe disk monitor data
	diskArgs := DescribeDisksArgs{
		InstanceId: TestInstanceId,
		RegionId:   instance.RegionId,
	}

	disks, _, err := client.DescribeDisks(&diskArgs)
	if err != nil {
		t.Fatalf("Failed to DescribeDisks for instance %s: %v", TestInstanceId, err)
	}

	diskId := disks[0].DiskId

	args := CreateSnapshotArgs{
		DiskId:       diskId,
		SnapshotName: "My_Test_Snapshot",
		Description:  "My Test Snapshot Description",
		ClientToken:  client.GenerateClientToken(),
	}

	snapshotId, err := client.CreateSnapshot(&args)
	if err != nil {
		t.Errorf("Failed to CreateSnapshot for disk %s: %v", diskId, err)
	}
	client.WaitForSnapShotReady(instance.RegionId, snapshotId, 0)

	err = client.DeleteSnapshot(snapshotId)
	if err != nil {
		t.Errorf("Failed to DeleteSnapshot for disk %s: %v", diskId, err)
	}

	t.Logf("Snapshot %s is deleted successfully.", snapshotId)

}
