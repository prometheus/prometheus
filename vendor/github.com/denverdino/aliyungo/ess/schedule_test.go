package ess

import (
	"testing"

	"github.com/denverdino/aliyungo/common"
)

func TestEssScalingScheduleCreationAndDeletion(t *testing.T) {

	if TestIAmRich == false {
		// Avoid payment
		return
	}
	client := NewTestClient(common.Region(RegionId))
	cArgs := CreateScheduledTaskArgs{
		RegionId:          common.Region(RegionId),
		ScheduledAction:   TestRuleArn,
		LaunchTime:        TestScheduleLaunchTime,
		ScheduledTaskName: "stn",
	}

	csc, err := client.CreateScheduledTask(&cArgs)
	if err != nil {
		t.Errorf("Failed to create scaling schedule %v", err)
	}
	taskId := csc.ScheduledTaskId
	t.Logf("Schedule task %s is created successfully.", taskId)

	mArgs := ModifyScheduledTaskArgs{
		RegionId:          common.Region(RegionId),
		ScheduledTaskId:   taskId,
		RecurrenceType:    RecurrenceType("Monthly"),
		RecurrenceValue:   "1-1",
		RecurrenceEndTime: TestScheduleLaunchTime,
		TaskEnabled:       true,
	}

	_, err = client.ModifyScheduledTask(&mArgs)
	if err != nil {
		t.Errorf("Failed to modify schedule task %v", err)
	}
	t.Logf("Schedule task %s is modify successfully.", taskId)

	sArgs := DescribeScheduledTasksArgs{
		RegionId:        common.Region(RegionId),
		ScheduledTaskId: []string{taskId},
	}
	sResp, _, err := client.DescribeScheduledTasks(&sArgs)
	if len(sResp) < 1 {
		t.Fatalf("Failed to describe schedule task %s", taskId)
	}

	task := sResp[0]
	t.Logf("Task: %++v  %v", task, err)

	dcArgs := DeleteScheduledTaskArgs{
		RegionId:        common.Region(RegionId),
		ScheduledTaskId: taskId,
	}

	_, err = client.DeleteScheduledTask(&dcArgs)
	if err != nil {
		t.Errorf("Failed to delete schedule task %v", err)
	}

	t.Logf("Task %s is deleted successfully.", taskId)

}
