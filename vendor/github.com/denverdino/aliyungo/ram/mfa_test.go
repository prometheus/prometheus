package ram

import (
	"strconv"
	"testing"
	"time"
)

var (
	uname               string
	serialNumber        string
	mFADeviceName       = strconv.FormatInt(time.Now().Unix(), 10)
	mFARequest          = MFARequest{VirtualMFADeviceName: mFADeviceName}
	authenticationCode1 = "Hello" //depends on your device
	authenticationCode2 = "World" //depends on your device
)

func TestCreateVirtualMFADevice(t *testing.T) {
	client := NewTestClient()
	resp, err := client.CreateVirtualMFADevice(mFARequest)
	if err != nil {
		t.Errorf("Failed to CreateVirtualMFADevice %v", err)
	}
	serialNumber = resp.VirtualMFADevice.SerialNumber
	t.Logf("pass CreateVirtualMFADevice %v", resp)
}

func TestListVirtualMFADevices(t *testing.T) {
	client := NewTestClient()
	resp, err := client.ListVirtualMFADevices()
	if err != nil {
		t.Errorf("Failed to ListVirtualMFADevices %v", err)
	}
	t.Logf("pass ListVirtualMFADevices %v", resp)
}

func TestBindMFADevice(t *testing.T) {
	client := NewTestClient()
	listParams := ListUserRequest{}
	resp, err := client.ListUsers(listParams)
	if err != nil {
		t.Errorf("Failed to ListUser %v", err)
		return
	}
	uname = resp.Users.User[0].UserName
	req := MFABindRequest{
		SerialNumber:        serialNumber,
		UserName:            uname,
		AuthenticationCode1: authenticationCode1,
		AuthenticationCode2: authenticationCode2,
	}
	response, err := client.BindMFADevice(req)
	if err != nil {
		t.Errorf("Failed to BindMFADevice %v", err)
	}
	t.Logf("pass BindMFADevice %v", response)
}

func TestGetUserMFAInfo(t *testing.T) {
	client := NewTestClient()
	resp, err := client.GetUserMFAInfo(UserQueryRequest{UserName: uname})
	if err != nil {
		t.Errorf("Failed to GetUserMFAInfo %v", err)
	}
	t.Logf("pass GetUserMFAInfo %v", resp)
}

func TestUnbindMFADevice(t *testing.T) {
	client := NewTestClient()
	response, err := client.UnbindMFADevice(UserQueryRequest{UserName: uname})
	if err != nil {
		t.Errorf("Failed to UnbindMFADevice %v", err)
	}
	t.Logf("pass UnbindMFADevice %v", response)
}

func TestDeleteVirtualMFADevice(t *testing.T) {
	client := NewTestClient()
	resp, err := client.DeleteVirtualMFADevice(MFADeleteRequest{MFADevice: MFADevice{SerialNumber: serialNumber}})
	if err != nil {
		t.Errorf("Failed to DeleteVirtualMFADevice %v", err)
	}
	t.Logf("pass DeleteVirtualMFADevice %v", resp)
}
