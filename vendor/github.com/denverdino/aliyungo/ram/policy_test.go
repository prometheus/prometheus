package ram

import (
	"encoding/json"
	//	"fmt"
	"testing"
)

var (
	policy_username  string
	policy_role_name string
	policy_name      string
	policy_document  = PolicyDocument{
		Statement: []PolicyItem{
			PolicyItem{
				Action:   "*",
				Effect:   "Allow",
				Resource: "*",
			},
		},
		Version: "1",
	}
	policy_req = PolicyRequest{
		PolicyName:  "unit_tesst_policy",
		Description: "nothing",
		PolicyType:  "Custom",
	}
	policy_group_name string
)

/*
	TODO maybe I need import ginko to my project
	BeforeEach and AfterClean is needed
*/

func TestCreatePolicy(t *testing.T) {
	var policyReq = policy_req
	document, err := json.Marshal(policy_document)
	if err != nil {
		t.Errorf("Failed to marshal document %v", err)
	}
	policyReq.PolicyDocument = string(document)
	client := NewTestClient()
	resp, err := client.CreatePolicy(policyReq)
	if err != nil {
		t.Errorf("Failed to CreatePolicy %v", err)
	}
	policy_name = resp.Policy.PolicyName
	t.Logf("pass CreatePolicy %+++v", resp)
}

func TestGetPolicy(t *testing.T) {
	var policyReq = policy_req
	policyReq.PolicyName = policy_name
	client := NewTestClient()
	resp, err := client.GetPolicy(policyReq)
	if err != nil {
		t.Errorf("Failed to GetPolicy %v", err)
	}
	t.Logf("pass GetPolicy %v", resp)
}

func TestAttachPolicyToUser(t *testing.T) {
	client := NewTestClient()
	listParams := ListUserRequest{}
	resp, err := client.ListUsers(listParams)
	if err != nil {
		t.Errorf("Failed to ListUser %v", err)
		return
	}
	policy_username = resp.Users.User[0].UserName
	attachPolicyRequest := AttachPolicyRequest{
		PolicyRequest: PolicyRequest{
			PolicyType: "Custom",
			PolicyName: policy_name,
		},
		UserName: policy_username,
	}
	attachResp, err := client.AttachPolicyToUser(attachPolicyRequest)
	if err != nil {
		t.Errorf("Failed to AttachPolicyToUser %v", err)
		return
	}
	t.Logf("pass AttachPolicyToUser %++v", attachResp)
}

func TestListPoliciesForUser(t *testing.T) {
	client := NewTestClient()
	userQuery := UserQueryRequest{
		UserName: policy_username,
	}
	resp, err := client.ListPoliciesForUser(userQuery)
	if err != nil {
		t.Errorf("Failed to ListPoliciesForUser %v", err)
		return
	}
	t.Logf("pass ListPoliciesForUser %++v", resp)
}

func TestDetachPolicyFromUser(t *testing.T) {
	client := NewTestClient()
	detachPolicyRequest := AttachPolicyRequest{
		PolicyRequest: PolicyRequest{
			PolicyType: "Custom",
			PolicyName: policy_name,
		},
		UserName: policy_username,
	}
	resp, err := client.DetachPolicyFromUser(detachPolicyRequest)
	if err != nil {
		t.Errorf("Failed to DetachPolicyFromUser %++v", err)
		return
	}
	t.Logf("pass DetachPolicyFromUser %++v", resp)
}

func TestAttachPolicyToRole(t *testing.T) {
	client := NewTestClient()
	resp, err := client.ListRoles()
	if err != nil {
		t.Errorf("Failed to ListRole %v", err)
		return
	}
	policy_role_name = resp.Roles.Role[0].RoleName
	attachPolicyRequest := AttachPolicyToRoleRequest{
		PolicyRequest: PolicyRequest{
			PolicyType: "Custom",
			PolicyName: policy_name,
		},
		RoleName: policy_role_name,
	}
	attachResp, err := client.AttachPolicyToRole(attachPolicyRequest)
	if err != nil {
		t.Errorf("Failed to AttachPolicyToRole %v", err)
		return
	}
	t.Logf("pass AttachPolicyToRole %++v", attachResp)
}

func TestListPoliciesForRole(t *testing.T) {
	client := NewTestClient()
	roleQuery := RoleQueryRequest{
		RoleName: policy_role_name,
	}
	resp, err := client.ListPoliciesForRole(roleQuery)
	if err != nil {
		t.Errorf("Failed to ListPoliciesForRole %v", err)
		return
	}
	t.Logf("pass ListPoliciesForRole %++v", resp)
}

func TestDetachPolicyFromRole(t *testing.T) {
	client := NewTestClient()
	detachPolicyRequest := AttachPolicyToRoleRequest{
		PolicyRequest: PolicyRequest{
			PolicyType: "Custom",
			PolicyName: policy_name,
		},
		RoleName: policy_role_name,
	}
	resp, err := client.DetachPolicyFromRole(detachPolicyRequest)
	if err != nil {
		t.Errorf("Failed to DetachPolicyFromRole %++v", err)
		return
	}
	t.Logf("pass DetachPolicyFromRole %++v", resp)
}

func TestAttachPolicyToGroup(t *testing.T) {
	client := NewTestClient()
	resp, err := client.ListGroup(GroupListRequest{})
	if err != nil {
		t.Errorf("Failed to ListGroup %v", err)
		return
	}
	policy_group_name = resp.Groups.Group[0].GroupName
	attachPolicyRequest := AttachPolicyToGroupRequest{
		PolicyRequest: PolicyRequest{
			PolicyType: "Custom",
			PolicyName: policy_name,
		},
		GroupName: policy_group_name,
	}
	attachResp, err := client.AttachPolicyToGroup(attachPolicyRequest)
	if err != nil {
		t.Errorf("Failed to AttachPolicyToGroup %v", err)
		return
	}
	t.Logf("pass AttachPolicyToGroup %++v", attachResp)
}

func TestListPoliciesForGroup(t *testing.T) {
	client := NewTestClient()
	groupQuery := GroupQueryRequest{
		GroupName: policy_group_name,
	}
	resp, err := client.ListPoliciesForGroup(groupQuery)
	if err != nil {
		t.Errorf("Failed to ListPoliciesForGroup %v", err)
		return
	}
	t.Logf("pass ListPoliciesForGroup %++v", resp)
}

func TEstListEntitiesForPolicy(t *testing.T) {
	client := NewTestClient()
	policyReq := PolicyRequest{
		PolicyType: "Custom",
		PolicyName: policy_name,
	}
	resp, err := client.ListEntitiesForPolicy(policyReq)
	if err != nil {
		t.Errorf("Failed to ListEntitiesForPolicy %++v", err)
		return
	}
	t.Logf("pass ListEntitiesForPolicy %++v", resp)
}

func TestDetachPolicyFromGroup(t *testing.T) {
	client := NewTestClient()
	detachPolicyRequest := AttachPolicyToGroupRequest{
		PolicyRequest: PolicyRequest{
			PolicyType: "Custom",
			PolicyName: policy_name,
		},
		GroupName: policy_group_name,
	}
	resp, err := client.DetachPolicyFromGroup(detachPolicyRequest)
	if err != nil {
		t.Errorf("Failed to DetachPolicyFromGroup %++v", err)
		return
	}
	t.Logf("pass DetachPolicyFromGroup %++v", resp)
}

func TestDeletePolicy(t *testing.T) {
	client := NewTestClient()
	policyReq := policy_req
	resp, err := client.DeletePolicy(policyReq)
	if err != nil {
		t.Errorf("Failed to DeletePolicy %v", err)
		return
	}
	t.Logf("pass DeletePolicy %++v", resp)
}
