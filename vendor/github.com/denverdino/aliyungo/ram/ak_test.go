package ram

import (
	"testing"
)

var (
	ram_ak_username string
	accessKeyId     string
	status          State
)

func TestCreateAccessKey(t *testing.T) {
	var ramUser User
	client := NewTestClient()
	listParams := ListUserRequest{}
	listUserResp, err := client.ListUsers(listParams)
	if err != nil {
		t.Errorf("Failed to list user %v", err)
	}
	if len(listUserResp.Users.User) == 0 {
		//TODO create user
		userResp, err := client.CreateUser(user)
		ramUser = userResp.User
		if err != nil {
			t.Errorf("Failed to create user %v", err)
		}
	} else {
		ramUser = listUserResp.Users.User[0]
	}
	//TODO 添加到公共变量中
	ram_ak_username = ramUser.UserName

	userQuery := UserQueryRequest{UserName: ram_ak_username}
	accessKeyResponse, err := client.CreateAccessKey(userQuery)
	//加入到公共变量中
	accessKeyId = accessKeyResponse.AccessKey.AccessKeyId
	status = accessKeyResponse.AccessKey.Status

	if err != nil {
		t.Errorf("Failed to create AccessKey %v", err)
	}
	t.Logf("pass create AccessKey %v", accessKeyResponse)
}

func TestUpdateAccessKey(t *testing.T) {
	client := NewTestClient()
	updateAccessKeyRequest := UpdateAccessKeyRequest{
		UserAccessKeyId: accessKeyId,
		UserName:        ram_ak_username,
	}
	if status == Active {
		updateAccessKeyRequest.Status = Inactive
	} else {
		updateAccessKeyRequest.Status = Active
	}

	accessKeyResponse, err := client.UpdateAccessKey(updateAccessKeyRequest)
	if err != nil {
		t.Errorf("Failed to UpdateAccessKey %v", err)
	}
	t.Logf("pass UpdateAccessKey %v", accessKeyResponse)
}

func TestListAccessKeys(t *testing.T) {
	client := NewTestClient()
	userQuery := UserQueryRequest{UserName: ram_ak_username}
	resp, err := client.ListAccessKeys(userQuery)
	if err != nil {
		t.Errorf("Failed to list ListAccessKeys %v", err)
	}
	t.Logf("pass ListAccessKeys %v", resp)
}

func TestDeleteAccessKey(t *testing.T) {
	client := NewTestClient()
	accessKeyRequest := UpdateAccessKeyRequest{
		UserAccessKeyId: accessKeyId,
		UserName:        ram_ak_username,
	}
	var commonResp RamCommonResponse
	commonResp, err := client.DeleteAccessKey(accessKeyRequest)
	if err != nil {
		t.Errorf("Failed to TestDeleteAccessKey %v", err)
	}
	t.Logf("pass DeleteAccessKey %v", commonResp)
}
