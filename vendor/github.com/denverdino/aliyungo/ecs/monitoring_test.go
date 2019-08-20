package ecs

import (
	"testing"
	"time"

	"github.com/denverdino/aliyungo/util"
)

func TestMonitoring(t *testing.T) {
	client := NewTestClient()
	//client.SetDebug(true)

	//Describe test instance
	instance, err := client.DescribeInstanceAttribute(TestInstanceId)
	if err != nil {
		t.Fatalf("Failed to describe instance %s: %v", TestInstanceId, err)
	}
	t.Logf("Instance: %++v  %v", instance, err)

	//Describe Instance Monitor Data
	now := time.Now().UTC()

	starting := time.Date(now.Year(), now.Month(), now.Day(), now.Hour()-2, now.Minute(), now.Second(), now.Nanosecond(), now.Location())
	ending := time.Date(now.Year(), now.Month(), now.Day(), now.Hour()-1, now.Minute(), now.Second(), now.Nanosecond(), now.Location())

	args := DescribeInstanceMonitorDataArgs{
		InstanceId: TestInstanceId,
		StartTime:  util.ISO6801Time(starting),
		EndTime:    util.ISO6801Time(ending),
	}

	monitorData, err := client.DescribeInstanceMonitorData(&args)

	if err != nil {
		t.Fatalf("Failed to describe monitoring data for instance %s: %v", TestInstanceId, err)
	}

	for _, data := range monitorData {
		t.Logf("Monitor Data: %++v", data)
	}
	//Describe EIP monitor data

	//Describe disk monitor data
	diskArgs := DescribeDisksArgs{
		InstanceId: TestInstanceId,
		RegionId:   instance.RegionId,
	}

	disks, _, err := client.DescribeDisks(&diskArgs)
	if err != nil {
		t.Fatalf("Failed to DescribeDisks for instance %s: %v", TestInstanceId, err)
	}

	for _, disk := range disks {
		args := DescribeDiskMonitorDataArgs{
			DiskId:    disk.DiskId,
			StartTime: util.ISO6801Time(starting),
			EndTime:   util.ISO6801Time(ending),
		}
		monitorData, _, err := client.DescribeDiskMonitorData(&args)

		if err != nil {
			t.Fatalf("Failed to describe monitoring data for disk %s: %v", disk.DiskId, err)
		}

		for _, data := range monitorData {
			t.Logf("Monitor Data: %++v", data)
		}
	}
}
