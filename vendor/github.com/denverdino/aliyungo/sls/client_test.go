package sls

import (
	"os"
	"testing"

	"github.com/denverdino/aliyungo/common"
)

var (
	TestAccessKeyId     = os.Getenv("AccessKeyId")
	TestAccessKeySecret = os.Getenv("AccessKeySecret")
	TestSecurityToken   = os.Getenv("SecurityToken")
	TestRegionID        = common.Region(os.Getenv("RegionId"))
	TestMachineGroup    = os.Getenv("MachineGroup")
	Region              = common.Hangzhou
	TestProjectName     = os.Getenv("ProjectName")
	TestLogstoreName    = "test-logstore"
)

func DefaultProject(t *testing.T) *Project {
	client := NewClient(Region, false, TestAccessKeyId, TestAccessKeySecret)
	err := client.CreateProject(TestProjectName, "description")
	if err != nil {
		if e, ok := err.(*Error); ok && e.Code != "ProjectAlreadyExist" {
			t.Fatalf("create project fail: %s", err.Error())
		}
	}
	p, err := client.Project(TestProjectName)
	if err != nil {
		t.Fatalf("get project fail: %s", err.Error())
	}
	//Create default logstore

	logstore := &Logstore{
		TTL:   2,
		Shard: 3,
		Name:  TestLogstoreName,
	}
	err = p.CreateLogstore(logstore)
	if err != nil {
		if e, ok := err.(*Error); ok && e.Code != "LogStoreAlreadyExist" {
			t.Fatalf("create logstore fail: %s", err.Error())
		}
	}

	return p
}

var testDebugClient *Client

func NewTestClientForDebug() *Client {
	if testDebugClient == nil {
		testDebugClient = NewClientForAssumeRole(TestRegionID, false, TestAccessKeyId, TestAccessKeySecret, TestSecurityToken)
		testDebugClient.SetDebug(true)
	}
	return testDebugClient
}
