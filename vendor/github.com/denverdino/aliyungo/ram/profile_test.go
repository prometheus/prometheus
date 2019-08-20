package ram

import (
	"testing"
)

var (
	password    = "hello&world"
	passwordNew = "hello&world&new"
	userName    string
)

func TestCreateLoginProfile(t *testing.T) {
	client := NewTestClient()
	listParams := ListUserRequest{}
	resp, err := client.ListUsers(listParams)
	if err != nil {
		t.Errorf("Failed to ListUser %v", err)
		return
	}
	userName = resp.Users.User[0].UserName
	profileRequest := ProfileRequest{
		UserName: userName,
		Password: password,
	}
	response, err := client.CreateLoginProfile(profileRequest)
	if err != nil {
		t.Errorf("Failed to CreateLoginProfile %v", err)
	}
	t.Logf("pass CreateLoginProfile %v", response)
}

func TestGetLoginProfile(t *testing.T) {
	client := NewTestClient()
	resp, err := client.GetLoginProfile(UserQueryRequest{UserName: userName})
	if err != nil {
		t.Errorf("Failed to GetLoginProfile %v", err)
	}
	t.Logf("pass GetLoginProfile %v", resp)
}

func TestUpdateLoginProfile(t *testing.T) {
	client := NewTestClient()
	profileRequest := ProfileRequest{
		UserName: userName,
		Password: passwordNew,
	}
	resp, err := client.UpdateLoginProfile(profileRequest)
	if err != nil {
		t.Errorf("Failed to UpdateLoginProfile %v", err)
	}
	t.Logf("pass UpdateLoginProfile %v", resp)
}

func TestDeleteLoginProfile(t *testing.T) {
	client := NewTestClient()
	resp, err := client.DeleteLoginProfile(UserQueryRequest{UserName: userName})
	if err != nil {
		t.Errorf("Failed to DeleteLoginProfile %v", err)
	}
	t.Logf("pass DeleteLoginProfile %v", resp)
}
