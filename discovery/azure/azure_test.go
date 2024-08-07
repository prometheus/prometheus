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
	"sort"
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
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
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

func TestAzureRefresh(t *testing.T) {
	tests := []struct {
		scenario   string
		vmResp     []armcompute.VirtualMachinesClientListAllResponse
		vmssResp   []armcompute.VirtualMachineScaleSetsClientListAllResponse
		vmssvmResp []armcompute.VirtualMachineScaleSetVMsClientListResponse
		expectedTG []*targetgroup.Group
	}{
		{
			scenario: "VMs, VMSS and VMSSVMs in Multiple Responses",
			vmResp: []armcompute.VirtualMachinesClientListAllResponse{
				{
					VirtualMachineListResult: armcompute.VirtualMachineListResult{
						Value: []*armcompute.VirtualMachine{
							defaultVMWithIDAndName(to.Ptr("/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/{resourceGroup}/providers/Microsoft.Compute/virtualMachine/vm1"), to.Ptr("vm1")),
							defaultVMWithIDAndName(to.Ptr("/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/{resourceGroup}/providers/Microsoft.Compute/virtualMachine/vm2"), to.Ptr("vm2")),
						},
					},
				},
				{
					VirtualMachineListResult: armcompute.VirtualMachineListResult{
						Value: []*armcompute.VirtualMachine{
							defaultVMWithIDAndName(to.Ptr("/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/{resourceGroup}/providers/Microsoft.Compute/virtualMachine/vm3"), to.Ptr("vm3")),
							defaultVMWithIDAndName(to.Ptr("/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/{resourceGroup}/providers/Microsoft.Compute/virtualMachine/vm4"), to.Ptr("vm4")),
						},
					},
				},
			},
			vmssResp: []armcompute.VirtualMachineScaleSetsClientListAllResponse{
				{
					VirtualMachineScaleSetListWithLinkResult: armcompute.VirtualMachineScaleSetListWithLinkResult{
						Value: []*armcompute.VirtualMachineScaleSet{
							{
								ID:       to.Ptr("/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/{resourceGroup}/providers/Microsoft.Compute/virtualMachineScaleSets/vmScaleSet1"),
								Name:     to.Ptr("vmScaleSet1"),
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
							defaultVMSSVMWithIDAndName(to.Ptr("/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/{resourceGroup}/providers/Microsoft.Compute/virtualMachineScaleSets/vmScaleSet1/virtualMachines/vmScaleSet1_vm1"), to.Ptr("vmScaleSet1_vm1")),
							defaultVMSSVMWithIDAndName(to.Ptr("/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/{resourceGroup}/providers/Microsoft.Compute/virtualMachineScaleSets/vmScaleSet1/virtualMachines/vmScaleSet1_vm2"), to.Ptr("vmScaleSet1_vm2")),
						},
					},
				},
			},
			expectedTG: []*targetgroup.Group{
				{
					Targets: []model.LabelSet{
						{
							"__address__":                         "10.0.0.1:80",
							"__meta_azure_machine_computer_name":  "computer_name",
							"__meta_azure_machine_id":             "/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/{resourceGroup}/providers/Microsoft.Compute/virtualMachine/vm1",
							"__meta_azure_machine_location":       "australiaeast",
							"__meta_azure_machine_name":           "vm1",
							"__meta_azure_machine_os_type":        "Linux",
							"__meta_azure_machine_private_ip":     "10.0.0.1",
							"__meta_azure_machine_resource_group": "{resourceGroup}",
							"__meta_azure_machine_size":           "size",
							"__meta_azure_machine_tag_prometheus": "",
							"__meta_azure_subscription_id":        "",
							"__meta_azure_tenant_id":              "",
						},
						{
							"__address__":                         "10.0.0.1:80",
							"__meta_azure_machine_computer_name":  "computer_name",
							"__meta_azure_machine_id":             "/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/{resourceGroup}/providers/Microsoft.Compute/virtualMachine/vm2",
							"__meta_azure_machine_location":       "australiaeast",
							"__meta_azure_machine_name":           "vm2",
							"__meta_azure_machine_os_type":        "Linux",
							"__meta_azure_machine_private_ip":     "10.0.0.1",
							"__meta_azure_machine_resource_group": "{resourceGroup}",
							"__meta_azure_machine_size":           "size",
							"__meta_azure_machine_tag_prometheus": "",
							"__meta_azure_subscription_id":        "",
							"__meta_azure_tenant_id":              "",
						},
						{
							"__address__":                         "10.0.0.1:80",
							"__meta_azure_machine_computer_name":  "computer_name",
							"__meta_azure_machine_id":             "/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/{resourceGroup}/providers/Microsoft.Compute/virtualMachine/vm3",
							"__meta_azure_machine_location":       "australiaeast",
							"__meta_azure_machine_name":           "vm3",
							"__meta_azure_machine_os_type":        "Linux",
							"__meta_azure_machine_private_ip":     "10.0.0.1",
							"__meta_azure_machine_resource_group": "{resourceGroup}",
							"__meta_azure_machine_size":           "size",
							"__meta_azure_machine_tag_prometheus": "",
							"__meta_azure_subscription_id":        "",
							"__meta_azure_tenant_id":              "",
						},
						{
							"__address__":                         "10.0.0.1:80",
							"__meta_azure_machine_computer_name":  "computer_name",
							"__meta_azure_machine_id":             "/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/{resourceGroup}/providers/Microsoft.Compute/virtualMachine/vm4",
							"__meta_azure_machine_location":       "australiaeast",
							"__meta_azure_machine_name":           "vm4",
							"__meta_azure_machine_os_type":        "Linux",
							"__meta_azure_machine_private_ip":     "10.0.0.1",
							"__meta_azure_machine_resource_group": "{resourceGroup}",
							"__meta_azure_machine_size":           "size",
							"__meta_azure_machine_tag_prometheus": "",
							"__meta_azure_subscription_id":        "",
							"__meta_azure_tenant_id":              "",
						},
						{
							"__address__":                         "10.0.0.1:80",
							"__meta_azure_machine_computer_name":  "computer_name",
							"__meta_azure_machine_id":             "/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/{resourceGroup}/providers/Microsoft.Compute/virtualMachineScaleSets/vmScaleSet1/virtualMachines/vmScaleSet1_vm1",
							"__meta_azure_machine_location":       "australiaeast",
							"__meta_azure_machine_name":           "vmScaleSet1_vm1",
							"__meta_azure_machine_os_type":        "Linux",
							"__meta_azure_machine_private_ip":     "10.0.0.1",
							"__meta_azure_machine_resource_group": "{resourceGroup}",
							"__meta_azure_machine_scale_set":      "vmScaleSet1",
							"__meta_azure_machine_size":           "size",
							"__meta_azure_machine_tag_prometheus": "",
							"__meta_azure_subscription_id":        "",
							"__meta_azure_tenant_id":              "",
						},
						{
							"__address__":                         "10.0.0.1:80",
							"__meta_azure_machine_computer_name":  "computer_name",
							"__meta_azure_machine_id":             "/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/{resourceGroup}/providers/Microsoft.Compute/virtualMachineScaleSets/vmScaleSet1/virtualMachines/vmScaleSet1_vm2",
							"__meta_azure_machine_location":       "australiaeast",
							"__meta_azure_machine_name":           "vmScaleSet1_vm2",
							"__meta_azure_machine_os_type":        "Linux",
							"__meta_azure_machine_private_ip":     "10.0.0.1",
							"__meta_azure_machine_resource_group": "{resourceGroup}",
							"__meta_azure_machine_scale_set":      "vmScaleSet1",
							"__meta_azure_machine_size":           "size",
							"__meta_azure_machine_tag_prometheus": "",
							"__meta_azure_subscription_id":        "",
							"__meta_azure_tenant_id":              "",
						},
					},
				},
			},
		},
	}
	for _, test := range tests {
		tc := test

		t.Run(tc.scenario, func(t *testing.T) {
			t.Parallel()
			azureSDConfig := &DefaultSDConfig
			networkInterface := defaultNetworkInterface()

			azureClient, err := createMockAzureClient(tc.vmResp, tc.vmssResp, tc.vmssvmResp, nil)
			if err != nil {
				t.Fatalf("unexpected error occurred while creating mock client: %v", err)
			}

			reg := prometheus.NewRegistry()
			refreshMetrics := discovery.NewRefreshMetrics(reg)
			metrics := azureSDConfig.NewDiscovererMetrics(reg, refreshMetrics)

			sd, err := NewDiscovery(azureSDConfig, nil, metrics)
			if err != nil {
				t.Fatalf("unexpected error occurred while creating new discovery: %v", err)
			}

			// Save the network details directly to cache since a mock server is not available.
			sd.cache.Set(*networkInterface.ID, networkInterface)
			defer sd.cache.Delete(*networkInterface.ID)

			tg, err := sd.refreshAzureClient(context.Background(), azureClient)
			if err != nil {
				t.Fatalf("unexpected error occurred while fetching target groups: %v", err)
			}

			sortTargetsByID(tg[0].Targets)
			require.Equal(t, tc.expectedTG, tg)
		})
	}
}

type mockAzureClient struct {
	networkInterface *armnetwork.Interface
	azureClient
}

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

func createMockAzureClient(vmResp []armcompute.VirtualMachinesClientListAllResponse, vmssResp []armcompute.VirtualMachineScaleSetsClientListAllResponse, vmssvmResp []armcompute.VirtualMachineScaleSetVMsClientListResponse, logger log.Logger) (client, error) {
	c := mockAzureClient{}

	mockVMServer := defaultMockVMServer(vmResp)
	mockVMSSServer := defaultMockVMSSServer(vmssResp)
	mockVMScaleSetVMServer := defaultMockVMSSVMServer(vmssvmResp)

	vmClient, err := armcompute.NewVirtualMachinesClient("fake-subscription-id", &azfake.TokenCredential{}, &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: fake.NewVirtualMachinesServerTransport(&mockVMServer),
		},
	})
	if err != nil {
		return &mockAzureClient{}, err
	}

	vmssClient, err := armcompute.NewVirtualMachineScaleSetsClient("fake-subscription-id", &azfake.TokenCredential{}, &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: fake.NewVirtualMachineScaleSetsServerTransport(&mockVMSSServer),
		},
	})
	if err != nil {
		return &mockAzureClient{}, err
	}

	vmssvmClient, err := armcompute.NewVirtualMachineScaleSetVMsClient("fake-subscription-id", &azfake.TokenCredential{}, &arm.ClientOptions{
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
	c.networkInterface = defaultNetworkInterface()
	c.logger = logger

	return &c, nil
}

func defaultMockVMServer(vmResp []armcompute.VirtualMachinesClientListAllResponse) fake.VirtualMachinesServer {
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

func defaultMockVMSSServer(vmssResp []armcompute.VirtualMachineScaleSetsClientListAllResponse) fake.VirtualMachineScaleSetsServer {
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

func defaultMockVMSSVMServer(vmssvmResp []armcompute.VirtualMachineScaleSetVMsClientListResponse) fake.VirtualMachineScaleSetVMsServer {
	fakeVirtualMachineScaleSetVMsServer := fake.VirtualMachineScaleSetVMsServer{
		NewListPager: func(resourceGroupName, virtualMachineScaleSetName string, options *armcompute.VirtualMachineScaleSetVMsClientListOptions) (resp azfake.PagerResponder[armcompute.VirtualMachineScaleSetVMsClientListResponse]) {
			for _, page := range vmssvmResp {
				resp.AddPage(http.StatusOK, page, nil)
			}
			return
		},
	}
	return fakeVirtualMachineScaleSetVMsServer
}

func defaultVMWithIDAndName(id, name *string) *armcompute.VirtualMachine {
	vmSize := armcompute.VirtualMachineSizeTypes("size")
	osType := armcompute.OperatingSystemTypesLinux
	defaultID := "/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/{resourceGroup}/providers/Microsoft.Compute/virtualMachine/testVM"
	defaultName := "testVM"

	if id == nil {
		id = &defaultID
	}
	if name == nil {
		name = &defaultName
	}

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

func defaultVMSSVMWithIDAndName(id, name *string) *armcompute.VirtualMachineScaleSetVM {
	vmSize := armcompute.VirtualMachineSizeTypes("size")
	osType := armcompute.OperatingSystemTypesLinux
	defaultID := "/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/{resourceGroup}/providers/Microsoft.Compute/virtualMachineScaleSets/testVMScaleSet/virtualMachines/testVM"
	defaultName := "testVM"

	if id == nil {
		id = &defaultID
	}
	if name == nil {
		name = &defaultName
	}

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

func defaultNetworkInterface() *armnetwork.Interface {
	networkID := "test"
	primary := true
	ipAddress := "10.0.0.1"
	return &armnetwork.Interface{
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
}

func sortTargetsByID(targets []model.LabelSet) {
	sort.Slice(targets, func(i, j int) bool {
		return targets[i]["__meta_azure_machine_id"] < targets[j]["__meta_azure_machine_id"]
	})
}
