package cms

import (
	"encoding/json"
	"testing"
)

func TestClient_CreateAlarm(t *testing.T) {
	client := NewTestClientForDebug()

	c := []map[string]string{
		{
			"port":       "SLB_PORT",
			"userId":     "USER_ID",
			"instanceId": "SLB_ID",
		},
	}

	b, _ := json.Marshal(c)

	alarm := CommonAlarmItem{
		Name:               "my-test-alarm-rule",
		Namespace:          "acs_slb_dashboard",
		MetricName:         "UnhealthyServerCount",
		Dimensions:         string(b),
		Statistics:         "Average",
		ComparisonOperator: ">",
		Threshold:          "0",
		ContactGroups:      "[\"云账号报警联系人\"]",
		NotifyType:         1,
	}
	args := &CreateAlarmArgs{
		CommonAlarmItem: alarm,
	}

	response, err := client.CreateAlarm(args)
	if err != nil {
		t.Fatalf("Failed to create alarm %++v", err)
	} else {
		t.Logf("Respose %++v", response)
	}
}

func TestClient_DeleteAlarm(t *testing.T) {
	client := NewTestClientForDebug()

	err := client.DeleteAlarm("c3ed7915-2606-467f-8512-9d83c28929d2")
	if err != nil {
		t.Fatalf("Failed to delete alarm %++v", err)
	} else {
		t.Logf("Delete success")
	}
}

func TestClient_ListAlarm(t *testing.T) {
	client := NewTestClientForDebug()

	args := &ListAlarmArgs{
		Name:      "my-test-alarm-rule",
		Namespace: "acs_slb_dashboard",
		IsEnable:  true,
	}
	response, err := client.ListAlarm(args)
	if err != nil {
		t.Fatalf("Failed to list alarm %++v", err)
	} else {
		t.Logf("Respose %++v", response)
	}
}

func TestClient_DisableAlarm(t *testing.T) {
	client := NewTestClientForDebug()

	err := client.DisableAlarm("778b15ea-53ff-4915-95a8-e63a801a18d4")
	if err != nil {
		t.Fatalf("Failed to disable alarm %++v", err)
	} else {
		t.Logf("Disable success")
	}
}

func TestClient_EnableAlarm(t *testing.T) {
	client := NewTestClientForDebug()

	err := client.EnableAlarm("778b15ea-53ff-4915-95a8-e63a801a18d4")
	if err != nil {
		t.Fatalf("Failed to enable alarm %++v", err)
	} else {
		t.Logf("Enable success")
	}
}

func TestClient_UpdateAlarm(t *testing.T) {
	client := NewTestClientForDebug()

	args := &UpdateAlarmArgs{
		Id:                 "778b15ea-53ff-4915-95a8-e63a801a18d4",
		Name:               "my-test-alarm-rule-00002-for-update",
		Statistics:         "Average",
		ComparisonOperator: ">",
		Threshold:          "0",
		ContactGroups:      "[\"云账号报警联系人\"]",
		NotifyType:         0,
	}

	err := client.UpdateAlarm(args)
	if err != nil {
		t.Fatalf("Failed to update alarm %++v", err)
	} else {
		t.Logf("Update success")
	}
}

func TestClient_ListAlarmHistory(t *testing.T) {
	client := NewTestClientForDebug()
	args := &ListAlarmHistoryArgs{}

	response, err := client.ListAlarmHistory(args)
	if err != nil {
		t.Fatalf("Failed to listAlarm history %++v", err)
	} else {
		t.Logf("Response is %++v", response)
	}
}

func TestClient_ListContactGroup(t *testing.T) {
	client := NewTestClientForDebug()
	client.SetSecurityToken(TestSecurityToken)

	args := &ListContactGroupArgs{}

	response, err := client.ListContactGroup(args)
	if err != nil {
		t.Fatalf("Failed to contact group %++v", err)
	} else {
		t.Logf("Response is %++v", response)
	}
}
