package sts

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"fmt"

	"github.com/denverdino/aliyungo/ecs"

	"github.com/denverdino/aliyungo/ram"
)

/*
  Please also set account id in env so that roles could be created test
	 AccessKeyId=YourAccessKeyId AccessKeySecret=YourAccessKeySecret AccountId=111111111 go test -v -run=AssumeRole
*/
var (
	accountId = os.Getenv("AccountId")
	roleName  = strconv.FormatInt(time.Now().Unix(), 10)

	princpal = ram.AssumeRolePolicyPrincpal{RAM: []string{"acs:ram::" + accountId + ":root"}}

	princpalPolicyDocument = ram.AssumeRolePolicyDocument{
		Statement: []ram.AssumeRolePolicyItem{
			ram.AssumeRolePolicyItem{Action: "sts:AssumeRole", Effect: "Allow", Principal: princpal},
		},
		Version: "1"}

	role = ram.RoleRequest{
		RoleName:                 roleName,
		AssumeRolePolicyDocument: getAssumeRolePolicyDocumentStr(),
		Description:              "this is a role for unit test purpose",
	}
)

var policyDocument = ram.PolicyDocument{
	Statement: []ram.PolicyItem{
		ram.PolicyItem{
			Action:   "oss:GetObject",
			Effect:   "Allow",
			Resource: "acs:oss:*:*:*/anyprefix",
		},
	},
	Version: "1",
}

func getAssumeRolePolicyDocumentStr() string {
	b, _ := json.Marshal(princpalPolicyDocument)
	return string(b)
}

func createAssumeRoleRequest(roleArn string) AssumeRoleRequest {
	document, _ := json.Marshal(policyDocument)
	return AssumeRoleRequest{
		RoleArn:         roleArn,
		RoleSessionName: "aliyungo-sts-unit-test",
		DurationSeconds: 3600,
		Policy:          string(document),
	}
}

func createPolicyDocument() *ram.PolicyDocument {
	return &ram.PolicyDocument{
		Statement: []ram.PolicyItem{
			ram.PolicyItem{
				Action:   "oss:GetObject",
				Effect:   "Allow",
				Resource: "acs:oss:*:*:*/*",
			},
		},
		Version: "1",
	}
}

func createPolicyReq() *ram.PolicyRequest {
	policyDocument := createPolicyDocument()
	document, _ := json.Marshal(*policyDocument)
	return &ram.PolicyRequest{
		PolicyName:     "sts-" + strconv.FormatInt(time.Now().Unix(), 10),
		PolicyType:     "Custom",
		PolicyDocument: string(document),
	}
}

func TestAssumeRole(t *testing.T) {

	//
	//1. create a role
	//
	ramClient := NewRAMTestClient()
	roleResp, err := ramClient.CreateRole(role)
	if err != nil {
		t.Errorf("Failed to CreateRole %v", err)
		return
	}

	//
	//2. create a policy to have the access to oss
	//
	policyResp, err := ramClient.CreatePolicy(*createPolicyReq())
	if err != nil {
		t.Errorf("Failed to CreatePolicy %v", err)
		return
	}

	//
	//2. attach a policy to this role
	//
	attachPolicyRequest := ram.AttachPolicyToRoleRequest{
		PolicyRequest: ram.PolicyRequest{
			PolicyType: "Custom",
			PolicyName: policyResp.Policy.PolicyName,
		},
		RoleName: roleResp.Role.RoleName,
	}

	_, err = ramClient.AttachPolicyToRole(attachPolicyRequest)
	if err != nil {
		t.Errorf("Failed to AttachPolicyToRole %v", err)
		return
	}

	//
	//CAUTION: Aliyun right now have a bug, once a role is created, if you assume this role immediately, it will always fail.
	//				You have to sleep for several seconds to work around this problem
	//
	time.Sleep(2 * time.Second)

	//
	//3. assume this role
	//
	client := NewTestClient()
	req := createAssumeRoleRequest(roleResp.Role.Arn)
	resp, err := client.AssumeRole(req)
	if err != nil {
		t.Errorf("Failed to AssumeRole %v", err)
		return
	}

	ecsClient := ecs.NewECSClientWithSecurityToken(resp.Credentials.AccessKeyId, resp.Credentials.AccessKeySecret, resp.Credentials.SecurityToken, "")
	_, err = ecsClient.DescribeRegions()
	if err != nil {
		t.Errorf("Failed to DescribeRegions %v", err)
		return
	}

	ramClient = ram.NewClientWithSecurityToken(resp.Credentials.AccessKeyId, resp.Credentials.AccessKeySecret, resp.Credentials.SecurityToken)
	req2 := ram.ListUserRequest{
		Marker:   "",
		MaxItems: 2,
	}
	_, err = ramClient.ListUsers(req2)
	if err != nil {
		t.Errorf("Failed to ListUsers %v", err)
		return
	}

	t.Logf("pass AssumeRole %v", resp)

}

func TestSTSClient_AssumeRole(t *testing.T) {
	client := NewTestClient()

	roleArn := os.Getenv("RoleArn")

	req := AssumeRoleRequest{
		RoleArn:         roleArn,
		RoleSessionName: fmt.Sprintf("commander-role-%d", time.Now().Unix()),
		DurationSeconds: 3600,
	}

	response, err := client.AssumeRole(req)
	if err != nil {
		t.Fatalf("%++v", err)
	} else {
		t.Logf("Response=%++v", response)
	}
}
