package ram

import (
	"strconv"
	"testing"
	"time"
)

var (
	groupName = strconv.FormatInt(time.Now().Unix(), 10)
	group     = GroupRequest{
		Group: Group{
			GroupName: groupName,
			Comments:  "no any comments",
		},
	}
	groupQuery   = GroupQueryRequest{GroupName: groupName}
	newGroupName = strconv.FormatInt(time.Now().Unix()+100, 10)
	groupNew     = GroupUpdateRequest{
		GroupName:    groupName,
		NewGroupName: newGroupName,
		NewComments:  "no any comments new",
	}
	uName string
)

func TestCreateGroup(t *testing.T) {
	client := NewTestClient()
	resp, err := client.CreateGroup(group)
	if err != nil {
		t.Errorf("Failed to CreateGroup %v", err)
	}
	t.Logf("pass CreateGroup %v", resp)
}

func TestGetGroup(t *testing.T) {
	client := NewTestClient()
	resp, err := client.GetGroup(groupQuery)
	if err != nil {
		t.Errorf("Failed to GetGroup %v", err)
	}
	t.Logf("pass GetGroup %v", resp)
}

func TestUpdateGroup(t *testing.T) {
	client := NewTestClient()
	resp, err := client.UpdateGroup(groupNew)
	if err != nil {
		t.Errorf("Failed to UpdateGroup %v", err)
	}
	t.Logf("pass UpdateGroup %v", resp)
}

func TestListGroup(t *testing.T) {
	client := NewTestClient()
	resp, err := client.ListGroup(GroupListRequest{})
	if err != nil {
		t.Errorf("Failed to ListGroup %v", err)
	}
	t.Logf("pass ListGroup %v", resp)
}

func TestAddUserToGroup(t *testing.T) {
	client := NewTestClient()
	listParams := ListUserRequest{}
	resp, err := client.ListUsers(listParams)
	if err != nil {
		t.Errorf("Failed to ListUser %v", err)
		return
	}
	uName = resp.Users.User[0].UserName
	addUserToGroupReq := UserRelateGroupRequest{
		UserName:  uName,
		GroupName: newGroupName,
	}
	response, err := client.AddUserToGroup(addUserToGroupReq)
	if err != nil {
		t.Errorf("Failed to AddUserToGroup %v", err)
	}
	t.Logf("pass AddUserToGroup %v", response)
}

func TestRemoveUserFromGroup(t *testing.T) {
	client := NewTestClient()
	removeUserToGroupReq := UserRelateGroupRequest{
		UserName:  uName,
		GroupName: newGroupName,
	}
	response, err := client.RemoveUserFromGroup(removeUserToGroupReq)
	if err != nil {
		t.Errorf("Failed to RemoveUserFromGroup %v", err)
	}
	t.Logf("pass RemoveUserFromGroup %v", response)
}

func TestListGroupsForUser(t *testing.T) {
	client := NewTestClient()
	response, err := client.ListGroupsForUser(UserQueryRequest{UserName: uName})
	if err != nil {
		t.Errorf("Failed to ListGroupsForUser %v", err)
	}
	t.Logf("pass ListGroupsForUser %v", response)
}

func TestListUsersForGroup(t *testing.T) {
	client := NewTestClient()
	groupQuery.GroupName = newGroupName
	response, err := client.ListUsersForGroup(groupQuery)
	if err != nil {
		t.Errorf("Failed to ListUsersForGroup %v", err)
	}
	t.Logf("pass ListUsersForGroup %v", response)
}

func TestDeleteGroup(t *testing.T) {
	client := NewTestClient()
	groupQuery.GroupName = newGroupName
	resp, err := client.DeleteGroup(groupQuery)
	if err != nil {
		t.Errorf("Failed to DeleteGroup %v", err)
	}
	t.Logf("pass DeleteGroup %v", resp)
}
