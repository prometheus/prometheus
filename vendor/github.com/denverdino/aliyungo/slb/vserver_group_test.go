package slb

import (
	"testing"
	"encoding/json"

	"github.com/denverdino/aliyungo/common"
	"fmt"
	"github.com/denverdino/aliyungo/util"
)

var client = NewClient("your accessId", "your accessId secret")
var loadBalancerId = "your lbid"
var region = common.Region("your region")
var serverId = "you vm id"
var deleteVServerGroupId = ""
var deleteVServerGroupIdList = make([]string, 0)

func TestCleanUp(t *testing.T) {
	err := client.DeleteLoadBalancerListener(loadBalancerId, 80)

	if err != nil {
		t.Errorf("Failed to DeleteLoadBalancerListener: %v", err)
	}

	err = client.DeleteLoadBalancerListener(loadBalancerId, 8080)

	if err != nil {
		t.Errorf("Failed to DeleteLoadBalancerListener: %v", err)
	}
}

func TestCreateVServerGroup(t *testing.T) {
	vBackendServer := VBackendServerType{
		ServerId: serverId,
		Port:     8080,
		Weight:   100,
	}
	vBackendServerSlice := make([]VBackendServerType, 0)
	vBackendServerSlice = append(vBackendServerSlice, vBackendServer)
	serversStr, err := json.Marshal(vBackendServerSlice)
	if err != nil {
		t.Error(err)
	}
	arg := &CreateVServerGroupArgs{
		LoadBalancerId:   loadBalancerId,
		VServerGroupName: "test",
		BackendServers:   string(serversStr),
		RegionId:         region,
	}
	response, err := client.CreateVServerGroup(arg)
	if err != nil {
		t.Error(err)
	} else {
		deleteVServerGroupId = response.VServerGroupId
		t.Log(response)
	}
}

func TestDescribeVServerGroups(t *testing.T) {
	arg := &DescribeVServerGroupsArgs{
		LoadBalancerId:loadBalancerId,
		RegionId:	region,
		IncludeListener: true,
		IncludeRule: true,
	}
	response, err := client.DescribeVServerGroups(arg)
	if err != nil {
		t.Error(err)
	} else {
		fmt.Printf(util.PrettyJson(response))
	}
}

func TestDescribeVServerGroupAttribute(t *testing.T) {
	arg := &DescribeVServerGroupAttributeArgs{
		VServerGroupId: deleteVServerGroupId,
		RegionId:       region,
	}
	response, err := client.DescribeVServerGroupAttribute(arg)
	if err != nil {
		t.Error(err)
	} else {
		t.Log(response)
	}

}

func TestSetVServerGroupAttribute(t *testing.T) {
	vBackendServer := VBackendServerType{
		ServerId: serverId,
		Port:     9090,
		Weight:   100,
	}
	vBackendServerSlice := make([]VBackendServerType, 0)
	vBackendServerSlice = append(vBackendServerSlice, vBackendServer)
	serversStr, err := json.Marshal(vBackendServerSlice)
	if err != nil {
		t.Error(err)
	}
	arg := &SetVServerGroupAttributeArgs{
		LoadBalancerId:   loadBalancerId,
		RegionId:         region,
		VServerGroupName: "test3",
		VServerGroupId:   deleteVServerGroupId,
		BackendServers:   string(serversStr),
	}
	response, err := client.SetVServerGroupAttribute(arg)
	if err != nil {
		t.Error(err)
	} else {
		t.Log(response)
	}
}

func TestAddVServerGroupBackendServers(t *testing.T) {
	vBackendServer := VBackendServerType{
		ServerId: serverId,
		Port:     9090,
		Weight:   100,
	}
	vBackendServerSlice := make([]VBackendServerType, 0)
	vBackendServerSlice = append(vBackendServerSlice, vBackendServer)
	serversStr, err := json.Marshal(vBackendServerSlice)
	if err != nil {
		t.Error(err)
	}
	arg := &AddVServerGroupBackendServersArgs{
		LoadBalancerId: loadBalancerId,
		RegionId:       region,
		VServerGroupId: deleteVServerGroupId,
		BackendServers: string(serversStr),
	}
	response, err := client.AddVServerGroupBackendServers(arg)
	if err != nil {
		t.Error(err)
	} else {
		t.Log(response)
	}
}

func TestModifyVServerGroupBackendServers(t *testing.T) {
	vBackendServer := VBackendServerType{
		ServerId: serverId,
		Port:     9091,
		Weight:   100,
	}
	vBackendServerSlice := make([]VBackendServerType, 0)
	vBackendServerSlice = append(vBackendServerSlice, vBackendServer)
	serversStr, err := json.Marshal(vBackendServerSlice)
	if err != nil {
		t.Error(err)
	}

	newvBackendServer := VBackendServerType{
		ServerId: serverId,
		Port:     9094,
		Weight:   100,
	}
	newvBackendServerSlice := make([]VBackendServerType, 0)
	newvBackendServerSlice = append(newvBackendServerSlice, newvBackendServer)
	newserversStr, err := json.Marshal(newvBackendServerSlice)
	if err != nil {
		t.Error(err)
	}

	arg := &ModifyVServerGroupBackendServersArgs{
		RegionId:          region,
		VServerGroupId:    deleteVServerGroupId,
		OldBackendServers: string(serversStr),
		NewBackendServers: string(newserversStr),
	}

	response, err := client.ModifyVServerGroupBackendServers(arg)
	if err != nil {
		t.Error(err)
	} else {
		t.Log(response)
	}
}

func TestRemoveVServerGroupBackendServers(t *testing.T) {
	vBackendServer := VBackendServerType{
		ServerId: serverId,
		Port:     80,
		Weight:   100,
	}
	vBackendServerSlice := make([]VBackendServerType, 0)
	vBackendServerSlice = append(vBackendServerSlice, vBackendServer)
	serversStr, err := json.Marshal(vBackendServerSlice)
	if err != nil {
		t.Error(err)
	}
	arg := &RemoveVServerGroupBackendServersArgs{
		LoadBalancerId: loadBalancerId,
		VServerGroupId: deleteVServerGroupId,
		BackendServers: string(serversStr),
		RegionId:       region,
	}

	response, err := client.RemoveVServerGroupBackendServers(arg)
	if err != nil {
		t.Error(err)
	} else {
		t.Log(response)
	}
}

func TestDescribeLoadBalancerTCPListenerAttribute(t *testing.T) {
	arg := &CreateLoadBalancerTCPListenerArgs{
		LoadBalancerId: loadBalancerId,
		ListenerPort:   8080,
		Bandwidth:      -1,
		VServerGroupId: deleteVServerGroupId,
	}
	err := client.CreateLoadBalancerTCPListener(arg)
	if err != nil {
		t.Errorf("Failed to CreateLoadBalancerTCPListenerArgs: %v", err)
	}
	response, err := client.DescribeLoadBalancerTCPListenerAttribute(loadBalancerId, 8080)
	if err != nil {
		t.Errorf("Failed to DescribeLoadBalancerTCPListenerAttribute: %v", err)
	}
	t.Logf("Listener: %++v", *response)
}

func TestDeleteVServerGroup(t *testing.T) {

	err := client.DeleteLoadBalancerListener(loadBalancerId, 80)

	if err != nil {
		t.Errorf("Failed to DeleteLoadBalancerListener: %v", err)
	}

	err = client.DeleteLoadBalancerListener(loadBalancerId, 8080)

	if err != nil {
		t.Errorf("Failed to DeleteLoadBalancerListener: %v", err)
	}

	for _, id := range deleteVServerGroupIdList {
		arg := &DeleteVServerGroupArgs{
			VServerGroupId: id,
			RegionId:       region,
		}
		response, err := client.DeleteVServerGroup(arg)
		if err != nil {
			t.Error(err)
		} else {
			t.Log(response)
		}
	}
}
