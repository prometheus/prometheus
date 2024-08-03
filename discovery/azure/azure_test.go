// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package azure

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	fake "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v4"
	cache "github.com/Code-Hex/go-generics-cache"
	"github.com/Code-Hex/go-generics-cache/policy/lru"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("github.com/Code-Hex/go-generics-cache.(*janitor).run.func1"),
	)
}

func TestMapFromVMWithEmptyTags(t *testing.T) {
	id := "test"
	name := "name"
	size := "size"
	vmSize := armcompute.VirtualMachineSizeTypes(size)
	osType := armcompute.OperatingSystemTypesLinux
	vmType := "type"
	location := "westeurope"
	computerName := "computer_name"
	networkProfile := armcompute.NetworkProfile{
		NetworkInterfaces: []*armcompute.NetworkInterfaceReference{},
	}
	properties := &armcompute.VirtualMachineProperties{
		OSProfile: &armcompute.OSProfile{
			ComputerName: &computerName,
		},
		StorageProfile: &armcompute.StorageProfile{
			OSDisk: &armcompute.OSDisk{
				OSType: &osType,
			},
		},
		NetworkProfile: &networkProfile,
		HardwareProfile: &armcompute.HardwareProfile{
			VMSize: &vmSize,
		},
	}

	testVM := armcompute.VirtualMachine{
		ID:         &id,
		Name:       &name,
		Type:       &vmType,
		Location:   &location,
		Tags:       nil,
		Properties: properties,
	}

	expectedVM := virtualMachine{
		ID:                id,
		Name:              name,
		ComputerName:      computerName,
		Type:              vmType,
		Location:          location,
		OsType:            "Linux",
		Tags:              map[string]*string{},
		NetworkInterfaces: []string{},
		Size:              size,
	}

	actualVM := mapFromVM(testVM)

	require.Equal(t, expectedVM, actualVM)
}

func TestVMToLabelSet(t *testing.T) {
	id := "/subscriptions/00000000-0000-0000-0000-000000000000/test"
	name := "name"
	size := "size"
	vmSize := armcompute.VirtualMachineSizeTypes(size)
	osType := armcompute.OperatingSystemTypesLinux
	vmType := "type"
	location := "westeurope"
	computerName := "computer_name"
	networkID := "/subscriptions/00000000-0000-0000-0000-000000000000/network1"
	ipAddress := "10.20.30.40"
	primary := true
	networkProfile := armcompute.NetworkProfile{
		NetworkInterfaces: []*armcompute.NetworkInterfaceReference{
			{
				ID:         &networkID,
				Properties: &armcompute.NetworkInterfaceReferenceProperties{Primary: &primary},
			},
		},
	}
	properties := &armcompute.VirtualMachineProperties{
		OSProfile: &armcompute.OSProfile{
			ComputerName: &computerName,
		},
		StorageProfile: &armcompute.StorageProfile{
			OSDisk: &armcompute.OSDisk{
				OSType: &osType,
			},
		},
		NetworkProfile: &networkProfile,
		HardwareProfile: &armcompute.HardwareProfile{
			VMSize: &vmSize,
		},
	}

	testVM := armcompute.VirtualMachine{
		ID:         &id,
		Name:       &name,
		Type:       &vmType,
		Location:   &location,
		Tags:       nil,
		Properties: properties,
	}

	expectedVM := virtualMachine{
		ID:                id,
		Name:              name,
		ComputerName:      computerName,
		Type:              vmType,
		Location:          location,
		OsType:            "Linux",
		Tags:              map[string]*string{},
		NetworkInterfaces: []string{networkID},
		Size:              size,
	}

	actualVM := mapFromVM(testVM)

	require.Equal(t, expectedVM, actualVM)

	cfg := DefaultSDConfig
	d := &Discovery{
		cfg:    &cfg,
		logger: log.NewNopLogger(),
		cache:  cache.New(cache.AsLRU[string, *armnetwork.Interface](lru.WithCapacity(5))),
	}
	network := armnetwork.Interface{
		Name: &networkID,
		Properties: &armnetwork.InterfacePropertiesFormat{
			Primary: &primary,
			IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
				{Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
					PrivateIPAddress: &ipAddress,
				}},
			},
		},
	}
	client := &mockAzureClient{
		networkInterface: &network,
	}
	labelSet, err := d.vmToLabelSet(context.Background(), client, actualVM)
	require.NoError(t, err)
	require.Len(t, labelSet, 11)
}

func TestMapFromVMWithEmptyOSType(t *testing.T) {
	id := "test"
	name := "name"
	size := "size"
	vmSize := armcompute.VirtualMachineSizeTypes(size)
	vmType := "type"
	location := "westeurope"
	computerName := "computer_name"
	networkProfile := armcompute.NetworkProfile{
		NetworkInterfaces: []*armcompute.NetworkInterfaceReference{},
	}
	properties := &armcompute.VirtualMachineProperties{
		OSProfile: &armcompute.OSProfile{
			ComputerName: &computerName,
		},
		StorageProfile: &armcompute.StorageProfile{
			OSDisk: &armcompute.OSDisk{},
		},
		NetworkProfile: &networkProfile,
		HardwareProfile: &armcompute.HardwareProfile{
			VMSize: &vmSize,
		},
	}

	testVM := armcompute.VirtualMachine{
		ID:         &id,
		Name:       &name,
		Type:       &vmType,
		Location:   &location,
		Tags:       nil,
		Properties: properties,
	}

	expectedVM := virtualMachine{
		ID:                id,
		Name:              name,
		ComputerName:      computerName,
		Type:              vmType,
		Location:          location,
		Tags:              map[string]*string{},
		NetworkInterfaces: []string{},
		Size:              size,
	}

	actualVM := mapFromVM(testVM)

	require.Equal(t, expectedVM, actualVM)
}

func TestMapFromVMWithTags(t *testing.T) {
	id := "test"
	name := "name"
	size := "size"
	vmSize := armcompute.VirtualMachineSizeTypes(size)
	osType := armcompute.OperatingSystemTypesLinux
	vmType := "type"
	location := "westeurope"
	computerName := "computer_name"
	tags := map[string]*string{
		"prometheus": new(string),
	}
	networkProfile := armcompute.NetworkProfile{
		NetworkInterfaces: []*armcompute.NetworkInterfaceReference{},
	}
	properties := &armcompute.VirtualMachineProperties{
		OSProfile: &armcompute.OSProfile{
			ComputerName: &computerName,
		},
		StorageProfile: &armcompute.StorageProfile{
			OSDisk: &armcompute.OSDisk{
				OSType: &osType,
			},
		},
		NetworkProfile: &networkProfile,
		HardwareProfile: &armcompute.HardwareProfile{
			VMSize: &vmSize,
		},
	}

	testVM := armcompute.VirtualMachine{
		ID:         &id,
		Name:       &name,
		Type:       &vmType,
		Location:   &location,
		Tags:       tags,
		Properties: properties,
	}

	expectedVM := virtualMachine{
		ID:                id,
		Name:              name,
		ComputerName:      computerName,
		Type:              vmType,
		Location:          location,
		OsType:            "Linux",
		Tags:              tags,
		NetworkInterfaces: []string{},
		Size:              size,
	}

	actualVM := mapFromVM(testVM)

	require.Equal(t, expectedVM, actualVM)
}

func TestMapFromVMScaleSetVMWithEmptyTags(t *testing.T) {
	id := "test"
	name := "name"
	size := "size"
	vmSize := armcompute.VirtualMachineSizeTypes(size)
	osType := armcompute.OperatingSystemTypesLinux
	vmType := "type"
	instanceID := "123"
	location := "westeurope"
	computerName := "computer_name"
	networkProfile := armcompute.NetworkProfile{
		NetworkInterfaces: []*armcompute.NetworkInterfaceReference{},
	}
	properties := &armcompute.VirtualMachineScaleSetVMProperties{
		OSProfile: &armcompute.OSProfile{
			ComputerName: &computerName,
		},
		StorageProfile: &armcompute.StorageProfile{
			OSDisk: &armcompute.OSDisk{
				OSType: &osType,
			},
		},
		NetworkProfile: &networkProfile,
		HardwareProfile: &armcompute.HardwareProfile{
			VMSize: &vmSize,
		},
	}

	testVM := armcompute.VirtualMachineScaleSetVM{
		ID:         &id,
		Name:       &name,
		Type:       &vmType,
		InstanceID: &instanceID,
		Location:   &location,
		Tags:       nil,
		Properties: properties,
	}

	scaleSet := "testSet"
	expectedVM := virtualMachine{
		ID:                id,
		Name:              name,
		ComputerName:      computerName,
		Type:              vmType,
		Location:          location,
		OsType:            "Linux",
		Tags:              map[string]*string{},
		NetworkInterfaces: []string{},
		ScaleSet:          scaleSet,
		InstanceID:        instanceID,
		Size:              size,
	}

	actualVM := mapFromVMScaleSetVM(testVM, scaleSet)

	require.Equal(t, expectedVM, actualVM)
}

func TestMapFromVMScaleSetVMWithEmptyOSType(t *testing.T) {
	id := "test"
	name := "name"
	size := "size"
	vmSize := armcompute.VirtualMachineSizeTypes(size)
	vmType := "type"
	instanceID := "123"
	location := "westeurope"
	computerName := "computer_name"
	networkProfile := armcompute.NetworkProfile{
		NetworkInterfaces: []*armcompute.NetworkInterfaceReference{},
	}
	properties := &armcompute.VirtualMachineScaleSetVMProperties{
		OSProfile: &armcompute.OSProfile{
			ComputerName: &computerName,
		},
		StorageProfile: &armcompute.StorageProfile{},
		NetworkProfile: &networkProfile,
		HardwareProfile: &armcompute.HardwareProfile{
			VMSize: &vmSize,
		},
	}

	testVM := armcompute.VirtualMachineScaleSetVM{
		ID:         &id,
		Name:       &name,
		Type:       &vmType,
		InstanceID: &instanceID,
		Location:   &location,
		Tags:       nil,
		Properties: properties,
	}

	scaleSet := "testSet"
	expectedVM := virtualMachine{
		ID:                id,
		Name:              name,
		ComputerName:      computerName,
		Type:              vmType,
		Location:          location,
		Tags:              map[string]*string{},
		NetworkInterfaces: []string{},
		ScaleSet:          scaleSet,
		InstanceID:        instanceID,
		Size:              size,
	}

	actualVM := mapFromVMScaleSetVM(testVM, scaleSet)

	require.Equal(t, expectedVM, actualVM)
}

func TestMapFromVMScaleSetVMWithTags(t *testing.T) {
	id := "test"
	name := "name"
	size := "size"
	vmSize := armcompute.VirtualMachineSizeTypes(size)
	osType := armcompute.OperatingSystemTypesLinux
	vmType := "type"
	instanceID := "123"
	location := "westeurope"
	computerName := "computer_name"
	tags := map[string]*string{
		"prometheus": new(string),
	}
	networkProfile := armcompute.NetworkProfile{
		NetworkInterfaces: []*armcompute.NetworkInterfaceReference{},
	}
	properties := &armcompute.VirtualMachineScaleSetVMProperties{
		OSProfile: &armcompute.OSProfile{
			ComputerName: &computerName,
		},
		StorageProfile: &armcompute.StorageProfile{
			OSDisk: &armcompute.OSDisk{
				OSType: &osType,
			},
		},
		NetworkProfile: &networkProfile,
		HardwareProfile: &armcompute.HardwareProfile{
			VMSize: &vmSize,
		},
	}

	testVM := armcompute.VirtualMachineScaleSetVM{
		ID:         &id,
		Name:       &name,
		Type:       &vmType,
		InstanceID: &instanceID,
		Location:   &location,
		Tags:       tags,
		Properties: properties,
	}

	scaleSet := "testSet"
	expectedVM := virtualMachine{
		ID:                id,
		Name:              name,
		ComputerName:      computerName,
		Type:              vmType,
		Location:          location,
		OsType:            "Linux",
		Tags:              tags,
		NetworkInterfaces: []string{},
		ScaleSet:          scaleSet,
		InstanceID:        instanceID,
		Size:              size,
	}

	actualVM := mapFromVMScaleSetVM(testVM, scaleSet)

	require.Equal(t, expectedVM, actualVM)
}

func TestNewAzureResourceFromID(t *testing.T) {
	for _, tc := range []struct {
		id       string
		expected *arm.ResourceID
	}{
		{
			id: "/subscriptions/SUBSCRIPTION_ID/resourceGroups/group/providers/PROVIDER/TYPE/name",
			expected: &arm.ResourceID{
				Name:              "name",
				ResourceGroupName: "group",
			},
		},
		{
			id: "/subscriptions/SUBSCRIPTION_ID/resourceGroups/group/providers/PROVIDER/TYPE/name/TYPE/h",
			expected: &arm.ResourceID{
				Name:              "h",
				ResourceGroupName: "group",
			},
		},
	} {
		actual, err := newAzureResourceFromID(tc.id, nil)
		require.NoError(t, err)
		require.Equal(t, tc.expected.Name, actual.Name)
		require.Equal(t, tc.expected.ResourceGroupName, actual.ResourceGroupName)
	}
}

func testAzureSDRefresh(vmResp []armcompute.VirtualMachinesClientListAllResponse, vmssResp []armcompute.VirtualMachineScaleSetsClientListAllResponse, vmssvmResp []armcompute.VirtualMachineScaleSetVMsClientListResponse, logger log.Logger) ([]*targetgroup.Group, error) {
	azureSDConfig := &DefaultSDConfig

	azureClient, err := createNewMockAzureClient(vmResp, vmssResp, vmssvmResp, logger)
	if err != nil {
		return nil, err
	}

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := azureSDConfig.NewDiscovererMetrics(reg, refreshMetrics)

	sd, err := NewDiscovery(azureSDConfig, nil, metrics)
	if err != nil {
		return nil, err
	}

	networkID := "test"
	primary := true
	ipAddress := "10.0.0.1"
	networkInterface := &armnetwork.Interface{
		ID: &networkID,
		Properties: &armnetwork.InterfacePropertiesFormat{
			Primary: &primary,
			IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
				{Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
					PrivateIPAddress: &ipAddress,
				}},
			},
		},
	}
	// Save the network details to cache
	sd.cache.Set(*networkInterface.ID, networkInterface)

	tg, err := sd.refreshAzureClient(azureClient, context.Background())
	if err != nil {
		return nil, err
	}

	return tg, nil
}

type mockAzureClient struct {
	networkInterface *armnetwork.Interface
	azureClient
}

type vmOptions struct {
	vmType     string
	location   string
	properties *armcompute.VirtualMachineProperties
	tags       map[string]*string
}

// It should be as simple as writing down the VMs and getting the Target groups.
// You know what I mean?

// This means we will have to refactor a lot here.m

func TestAzureRefresh(t *testing.T) {
	t.Parallel()
	tests := []struct {
		scenario   string
		vmResp     []armcompute.VirtualMachinesClientListAllResponse
		vmssResp   []armcompute.VirtualMachineScaleSetsClientListAllResponse
		vmssvmResp []armcompute.VirtualMachineScaleSetVMsClientListResponse
		expectedTG []*targetgroup.Group
	}{
		{
			scenario: "Test Multiple VMs, VMSS and VMSSVMs in Multiple Responses",
			vmResp: []armcompute.VirtualMachinesClientListAllResponse{
				{
					VirtualMachineListResult: armcompute.VirtualMachineListResult{
						Value: []*armcompute.VirtualMachine{
							defaultVMWithIdAndName(to.Ptr("/subscriptions/bd5d2ee2-2989-4589-95fb-605e6167c89/resourceGroups/test1/providers/Microsoft.Compute/virtualMachine/vm1"), to.Ptr("vm1")),
							defaultVMWithIdAndName(to.Ptr("/subscriptions/bd5d2ee2-2989-4589-95fb-605e6167c89/resourceGroups/test1/providers/Microsoft.Compute/virtualMachine/vm2"), to.Ptr("vm2")),
						},
					},
				},
				{
					VirtualMachineListResult: armcompute.VirtualMachineListResult{
						Value: []*armcompute.VirtualMachine{
							defaultVMWithIdAndName(to.Ptr("/subscriptions/bd5d2ee2-2989-4589-95fb-605e6167c89/resourceGroups/test1/providers/Microsoft.Compute/virtualMachine/vm3"), to.Ptr("vm3")),
							defaultVMWithIdAndName(to.Ptr("/subscriptions/bd5d2ee2-2989-4589-95fb-605e6167c89/resourceGroups/test1/providers/Microsoft.Compute/virtualMachine/vm4"), to.Ptr("vm4")),
						},
					},
				},
			},
			vmssResp: []armcompute.VirtualMachineScaleSetsClientListAllResponse{
				{
					VirtualMachineScaleSetListWithLinkResult: armcompute.VirtualMachineScaleSetListWithLinkResult{
						Value: []*armcompute.VirtualMachineScaleSet{
							{
								ID:       to.Ptr("/subscriptions/bd5d2ee2-2989-4589-95fb-605e6167c89/resourceGroups/test1/providers/Microsoft.Compute/virtualMachineScaleSets/vmScaleSet1"),
								Name:     to.Ptr("vmScaleSet1"),
								Location: to.Ptr("australiaeast"),
								Type:     to.Ptr("Microsoft.Compute/virtualMachineScaleSets"),
							},
							{
								ID:       to.Ptr("/subscriptions/bd5d2ee2-2989-4589-95fb-605e6167c89/resourceGroups/test1/providers/Microsoft.Compute/virtualMachineScaleSets/vmScaleSet2"),
								Name:     to.Ptr("vmScaleSet2"),
								Location: to.Ptr("australiaeast"),
								Type:     to.Ptr("Microsoft.Compute/virtualMachineScaleSets"),
							},
						},
					},
				},
			},
			vmssvmResp: []armcompute.VirtualMachineScaleSetVMsClientListResponse{
				{
					VirtualMachineScaleSetVMListResult: armcompute.VirtualMachineScaleSetVMListResult{
						Value: []*armcompute.VirtualMachineScaleSetVM{
							defaultVMSSVMWithIdAndName(to.Ptr("/subscriptions/bd5d2ee2-2989-4589-95fb-605e6167c89/resourceGroups/test1/providers/Microsoft.Compute/virtualMachineScaleSets/vmScaleSet1/virtualMachines/vmScaleSet1_777e48b7"), to.Ptr("vmScaleSet1_777e48b7")),
							defaultVMSSVMWithIdAndName(to.Ptr("/subscriptions/bd5d2ee2-2989-4589-95fb-605e6167c89/resourceGroups/test1/providers/Microsoft.Compute/virtualMachineScaleSets/vmScaleSet1/virtualMachines/vmScaleSet1_9f135d64"), to.Ptr("vmScaleSet1_9f135d64")),
						},
					},
				},
				{
					VirtualMachineScaleSetVMListResult: armcompute.VirtualMachineScaleSetVMListResult{
						Value: []*armcompute.VirtualMachineScaleSetVM{
							defaultVMSSVMWithIdAndName(to.Ptr("/subscriptions/bd5d2ee2-2989-4589-95fb-605e6167c89/resourceGroups/test1/providers/Microsoft.Compute/virtualMachineScaleSets/vmScaleSet2/virtualMachines/vmScaleSet2_777e48b7"), to.Ptr("vmScaleSet2_777e48b7")),
							defaultVMSSVMWithIdAndName(to.Ptr("/subscriptions/bd5d2ee2-2989-4589-95fb-605e6167c89/resourceGroups/test1/providers/Microsoft.Compute/virtualMachineScaleSets/vmScaleSet2/virtualMachines/vmScaleSet2_9f135d64"), to.Ptr("vmScaleSet2_9f135d64")),
						},
					},
				},
			},
		},
	}
	for _, test := range tests {
		tc := test
		t.Run(tc.scenario, func(t *testing.T) {
			_, err := testAzureSDRefresh(tc.vmResp, tc.vmssResp, tc.vmssvmResp, log.NewNopLogger())
			if err != nil {
				t.Fatalf("unexpected error occurred while fetching target groups: %v", err)
			}

			// Check the returned TGs against expected TGs
		})
	}
}

func createNewMockAzureClient(vmResp []armcompute.VirtualMachinesClientListAllResponse, vmssResp []armcompute.VirtualMachineScaleSetsClientListAllResponse, vmssvmResp []armcompute.VirtualMachineScaleSetVMsClientListResponse, logger log.Logger) (client, error) {
	networkID := "/subscriptions/00000000-0000-0000-0000-000000000000/network1"
	ipAddress := "10.20.30.40"
	primary := true

	var vmClient *armcompute.VirtualMachinesClient
	var vmssClient *armcompute.VirtualMachineScaleSetsClient
	var vmssvmClient *armcompute.VirtualMachineScaleSetVMsClient

	var err error

	c := mockAzureClient{}

	// Create mock servers.
	mockVMServer := newDefaultMockVMServer(vmResp)
	mockVMSSServer := newDefaultMockVMSSServer(vmssResp)
	mockVMScaleSetVMServer := newDefaultMockVMSSVMServer(vmssvmResp)

	// Connect our client with the mock servers using mock transports (round-trippers).
	vmClient, err = armcompute.NewVirtualMachinesClient("fake-subscription-id", &azfake.TokenCredential{}, &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: fake.NewVirtualMachinesServerTransport(&mockVMServer),
		},
	})
	if err != nil {
		return &mockAzureClient{}, err
	}

	vmssClient, err = armcompute.NewVirtualMachineScaleSetsClient("fake-subscription-id", &azfake.TokenCredential{}, &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: fake.NewVirtualMachineScaleSetsServerTransport(&mockVMSSServer),
		},
	})
	if err != nil {
		return &mockAzureClient{}, err
	}

	vmssvmClient, err = armcompute.NewVirtualMachineScaleSetVMsClient("fake-subscription-id", &azfake.TokenCredential{}, &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: fake.NewVirtualMachineScaleSetVMsServerTransport(&mockVMScaleSetVMServer),
		},
	})
	if err != nil {
		return &mockAzureClient{}, err
	}

	c.vm = vmClient
	c.vmss = vmssClient
	c.vmssvm = vmssvmClient
	c.networkInterface = &armnetwork.Interface{
		Name: &networkID,
		Properties: &armnetwork.InterfacePropertiesFormat{
			Primary: &primary,
			IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
				{Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
					PrivateIPAddress: &ipAddress,
				}},
			},
		},
	}
	c.logger = logger

	return &c, nil
}

func defaultVMWithIdAndName(id *string, name *string) *armcompute.VirtualMachine {
	vmSize := armcompute.VirtualMachineSizeTypes("size")
	osType := armcompute.OperatingSystemTypesLinux

	return &armcompute.VirtualMachine{
		ID:       id,
		Name:     name,
		Type:     to.Ptr("Microsoft.Compute/virtualMachines"),
		Location: to.Ptr("australiaeast"),
		Properties: &armcompute.VirtualMachineProperties{
			OSProfile: &armcompute.OSProfile{
				ComputerName: to.Ptr("computer_name"),
			},
			StorageProfile: &armcompute.StorageProfile{
				OSDisk: &armcompute.OSDisk{
					OSType: &osType,
				},
			},
			NetworkProfile: &armcompute.NetworkProfile{
				NetworkInterfaces: []*armcompute.NetworkInterfaceReference{
					{
						ID: to.Ptr("test"),
					},
				},
			},
			HardwareProfile: &armcompute.HardwareProfile{
				VMSize: &vmSize,
			},
		},
		Tags: map[string]*string{
			"prometheus": new(string),
		},
	}
}

// we need to create a mock server and basically get data from it. lets write a function which creates a mock server

func newDefaultMockVMServer(vmResp []armcompute.VirtualMachinesClientListAllResponse) fake.VirtualMachinesServer {
	fakeVirtualMachinesServer := fake.VirtualMachinesServer{
		NewListAllPager: func(options *armcompute.VirtualMachinesClientListAllOptions) (resp azfake.PagerResponder[armcompute.VirtualMachinesClientListAllResponse]) {
			for _, page := range vmResp {
				resp.AddPage(http.StatusOK, page, nil)
			}
			return
		},
	}
	return fakeVirtualMachinesServer
}

func newDefaultMockVMSSServer(vmssResp []armcompute.VirtualMachineScaleSetsClientListAllResponse) fake.VirtualMachineScaleSetsServer {
	fakeVirtualMachineScaleSetsServer := fake.VirtualMachineScaleSetsServer{
		NewListAllPager: func(options *armcompute.VirtualMachineScaleSetsClientListAllOptions) (resp azfake.PagerResponder[armcompute.VirtualMachineScaleSetsClientListAllResponse]) {
			for _, page := range vmssResp {
				resp.AddPage(http.StatusOK, page, nil)
			}
			return
		},
	}
	return fakeVirtualMachineScaleSetsServer
}

func defaultVMSSVMWithIdAndName(id *string, name *string) *armcompute.VirtualMachineScaleSetVM {
	vmSize := armcompute.VirtualMachineSizeTypes("size")
	osType := armcompute.OperatingSystemTypesLinux

	return &armcompute.VirtualMachineScaleSetVM{
		ID:         id,
		Name:       name,
		Type:       to.Ptr("Microsoft.Compute/virtualMachines"),
		InstanceID: to.Ptr("123"),
		Location:   to.Ptr("australiaeast"),
		Properties: &armcompute.VirtualMachineScaleSetVMProperties{
			OSProfile: &armcompute.OSProfile{
				ComputerName: to.Ptr("computer_name"),
			},
			StorageProfile: &armcompute.StorageProfile{
				OSDisk: &armcompute.OSDisk{
					OSType: &osType,
				},
			},
			NetworkProfile: &armcompute.NetworkProfile{
				NetworkInterfaces: []*armcompute.NetworkInterfaceReference{
					{ID: to.Ptr("test")},
				},
			},
			HardwareProfile: &armcompute.HardwareProfile{
				VMSize: &vmSize,
			},
		},
		Tags: map[string]*string{
			"prometheus": new(string),
		},
	}
}

func newDefaultMockVMSSVMServer(vmssvmResp []armcompute.VirtualMachineScaleSetVMsClientListResponse) fake.VirtualMachineScaleSetVMsServer {
	fakeVirtualMachineScaleSetVMsServer := fake.VirtualMachineScaleSetVMsServer{
		NewListPager: func(resourceGroupName string, virtualMachineScaleSetName string, options *armcompute.VirtualMachineScaleSetVMsClientListOptions) (resp azfake.PagerResponder[armcompute.VirtualMachineScaleSetVMsClientListResponse]) {
			for _, page := range vmssvmResp {
				resp.AddPage(http.StatusOK, page, nil)
			}
			return
		},
	}
	return fakeVirtualMachineScaleSetVMsServer
}

// Mocking the interfaces client is not as straightforward as the others, for our
// purposes this implementation works just fine.
func (m *mockAzureClient) getVMNetworkInterfaceByID(ctx context.Context, networkInterfaceID string) (*armnetwork.Interface, error) {
	if networkInterfaceID == "" {
		return nil, fmt.Errorf("parameter networkInterfaceID cannot be empty")
	}
	return m.networkInterface, nil
}

func (m *mockAzureClient) getVMScaleSetVMNetworkInterfaceByID(ctx context.Context, networkInterfaceID, scaleSetName, instanceID string) (*armnetwork.Interface, error) {
	if scaleSetName == "" {
		return nil, fmt.Errorf("parameter virtualMachineScaleSetName cannot be empty")
	}
	return m.networkInterface, nil
}
