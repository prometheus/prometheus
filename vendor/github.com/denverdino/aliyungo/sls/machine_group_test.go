package sls

import (
	"fmt"
	"testing"
)

func TestListMachineGroups(t *testing.T) {
	p := DefaultProject(t)
	groups, err := p.ListMachineGroup(0, 100)
	if err != nil {
		fmt.Printf("Error in list groups %s \n", err)
		t.FailNow()
	}

	fmt.Println(groups)
}

func TestCreateMachineGroup(t *testing.T) {
	p := DefaultProject(t)
	groupName := "testGroup"
	group := &MachineGroup{
		Name:                groupName,
		MachineIdentifyType: "ip",
		Attribute:           &GroupAttribute{},
		MachineList: []string{
			"127.0.0.1",
			"127.0.0.2",
		},
	}
	err := p.CreateMachineGroup(group)
	if err != nil {
		if e, ok := err.(*Error); ok && e.Code != "GroupAlreadyExist" {
			t.Fatalf("Create machine error: %v", err)
		}
	}

	mg, err := p.MachineGroup(groupName)
	if err != nil {
		t.Fatalf("Find machine error: %v", err)
	}

	_, err = p.ListMachines("testGroup", 0, 100)
	if err != nil {
		t.Fatalf("List machine error: %v", err)
	}

	//Update MachineGroup
	mg.MachineList = append(mg.MachineList, "127.0.0.3")
	if err = p.UpdateMachineGroup(mg); err != nil {
		t.Fatalf("Update machine error: %v", err)
	}

	if err = p.DeleteMachineGroup(groupName); err != nil {
		t.Fatalf("Delete machine error: %v", err)
	}
}

func TestClient_ListMachineGroups(t *testing.T) {
	client := NewTestClientForDebug()
	p, err := client.Project(TestProjectName)
	if err != nil {
		t.Fatalf("get project fail: %++v", err)
	}

	machines, err := p.ListMachines(TestMachineGroup, 0, 100)
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		for index, machine := range machines.Machines {
			t.Logf("Machines[%d] = %++v", index, machine)
		}

	}
}
