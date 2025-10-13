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
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	fake "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v4"
	fakenetwork "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v4/fake"
	cache "github.com/Code-Hex/go-generics-cache"
	"github.com/Code-Hex/go-generics-cache/policy/lru"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const defaultMockNetworkID string = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network/networkInterfaces/{networkInterfaceName}"

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
	ipAddress := "10.20.30.40"
	primary := true
	networkProfile := armcompute.NetworkProfile{
		NetworkInterfaces: []*armcompute.NetworkInterfaceReference{
			{
				ID:         to.Ptr(defaultMockNetworkID),
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
		NetworkInterfaces: []string{defaultMockNetworkID},
		Size:              size,
	}

	actualVM := mapFromVM(testVM)

	require.Equal(t, expectedVM, actualVM)

	cfg := DefaultSDConfig
	d := &Discovery{
		cfg:    &cfg,
		logger: promslog.NewNopLogger(),
		cache:  cache.New(cache.AsLRU[string, *armnetwork.Interface](lru.WithCapacity(5))),
	}
	network := armnetwork.Interface{
		Name: to.Ptr(defaultMockNetworkID),
		ID:   to.Ptr(defaultMockNetworkID),
		Properties: &armnetwork.InterfacePropertiesFormat{
			Primary: &primary,
			IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
				{Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
					PrivateIPAddress: &ipAddress,
				}},
			},
		},
	}

	client := createMockAzureClient(t, nil, nil, nil, network, nil)

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
		scenario       string
		vmResp         []armcompute.VirtualMachinesClientListAllResponse
		vmssResp       []armcompute.VirtualMachineScaleSetsClientListAllResponse
		vmssvmResp     []armcompute.VirtualMachineScaleSetVMsClientListResponse
		interfacesResp armnetwork.Interface
		expectedTG     []*targetgroup.Group
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
			interfacesResp: armnetwork.Interface{
				ID: to.Ptr(defaultMockNetworkID),
				Properties: &armnetwork.InterfacePropertiesFormat{
					Primary: to.Ptr(true),
					IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
						{Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
							PrivateIPAddress: to.Ptr("10.0.0.1"),
						}},
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
	for _, tc := range tests {
		t.Run(tc.scenario, func(t *testing.T) {
			t.Parallel()
			azureSDConfig := &DefaultSDConfig

			azureClient := createMockAzureClient(t, tc.vmResp, tc.vmssResp, tc.vmssvmResp, tc.interfacesResp, nil)

			reg := prometheus.NewRegistry()
			refreshMetrics := discovery.NewRefreshMetrics(reg)
			metrics := azureSDConfig.NewDiscovererMetrics(reg, refreshMetrics)

			sd, err := NewDiscovery(azureSDConfig, nil, metrics)
			require.NoError(t, err)

			tg, err := sd.refreshAzureClient(context.Background(), azureClient)
			require.NoError(t, err)

			sortTargetsByID(tg[0].Targets)
			require.Equal(t, tc.expectedTG, tg)
		})
	}
}

type mockAzureClient struct {
	azureClient
}

func createMockAzureClient(t *testing.T, vmResp []armcompute.VirtualMachinesClientListAllResponse, vmssResp []armcompute.VirtualMachineScaleSetsClientListAllResponse, vmssvmResp []armcompute.VirtualMachineScaleSetVMsClientListResponse, interfaceResp armnetwork.Interface, logger *slog.Logger) client {
	t.Helper()
	mockVMServer := defaultMockVMServer(vmResp)
	mockVMSSServer := defaultMockVMSSServer(vmssResp)
	mockVMScaleSetVMServer := defaultMockVMSSVMServer(vmssvmResp)
	mockInterfaceServer := defaultMockInterfaceServer(interfaceResp)

	vmClient, err := armcompute.NewVirtualMachinesClient("fake-subscription-id", &azfake.TokenCredential{}, &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: fake.NewVirtualMachinesServerTransport(&mockVMServer),
		},
	})
	require.NoError(t, err)

	vmssClient, err := armcompute.NewVirtualMachineScaleSetsClient("fake-subscription-id", &azfake.TokenCredential{}, &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: fake.NewVirtualMachineScaleSetsServerTransport(&mockVMSSServer),
		},
	})
	require.NoError(t, err)

	vmssvmClient, err := armcompute.NewVirtualMachineScaleSetVMsClient("fake-subscription-id", &azfake.TokenCredential{}, &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: fake.NewVirtualMachineScaleSetVMsServerTransport(&mockVMScaleSetVMServer),
		},
	})
	require.NoError(t, err)

	interfacesClient, err := armnetwork.NewInterfacesClient("fake-subscription-id", &azfake.TokenCredential{}, &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: fakenetwork.NewInterfacesServerTransport(&mockInterfaceServer),
		},
	})
	require.NoError(t, err)

	return &mockAzureClient{
		azureClient: azureClient{
			vm:     vmClient,
			vmss:   vmssClient,
			vmssvm: vmssvmClient,
			nic:    interfacesClient,
			logger: logger,
		},
	}
}

func defaultMockInterfaceServer(interfaceResp armnetwork.Interface) fakenetwork.InterfacesServer {
	return fakenetwork.InterfacesServer{
		Get: func(context.Context, string, string, *armnetwork.InterfacesClientGetOptions) (resp azfake.Responder[armnetwork.InterfacesClientGetResponse], errResp azfake.ErrorResponder) {
			resp.SetResponse(http.StatusOK, armnetwork.InterfacesClientGetResponse{Interface: interfaceResp}, nil)
			return
		},
		GetVirtualMachineScaleSetNetworkInterface: func(context.Context, string, string, string, string, *armnetwork.InterfacesClientGetVirtualMachineScaleSetNetworkInterfaceOptions) (resp azfake.Responder[armnetwork.InterfacesClientGetVirtualMachineScaleSetNetworkInterfaceResponse], errResp azfake.ErrorResponder) {
			resp.SetResponse(http.StatusOK, armnetwork.InterfacesClientGetVirtualMachineScaleSetNetworkInterfaceResponse{Interface: interfaceResp}, nil)
			return
		},
	}
}

func defaultMockVMServer(vmResp []armcompute.VirtualMachinesClientListAllResponse) fake.VirtualMachinesServer {
	return fake.VirtualMachinesServer{
		NewListAllPager: func(*armcompute.VirtualMachinesClientListAllOptions) (resp azfake.PagerResponder[armcompute.VirtualMachinesClientListAllResponse]) {
			for _, page := range vmResp {
				resp.AddPage(http.StatusOK, page, nil)
			}
			return
		},
	}
}

func defaultMockVMSSServer(vmssResp []armcompute.VirtualMachineScaleSetsClientListAllResponse) fake.VirtualMachineScaleSetsServer {
	return fake.VirtualMachineScaleSetsServer{
		NewListAllPager: func(*armcompute.VirtualMachineScaleSetsClientListAllOptions) (resp azfake.PagerResponder[armcompute.VirtualMachineScaleSetsClientListAllResponse]) {
			for _, page := range vmssResp {
				resp.AddPage(http.StatusOK, page, nil)
			}
			return
		},
	}
}

func defaultMockVMSSVMServer(vmssvmResp []armcompute.VirtualMachineScaleSetVMsClientListResponse) fake.VirtualMachineScaleSetVMsServer {
	return fake.VirtualMachineScaleSetVMsServer{
		NewListPager: func(_, _ string, _ *armcompute.VirtualMachineScaleSetVMsClientListOptions) (resp azfake.PagerResponder[armcompute.VirtualMachineScaleSetVMsClientListResponse]) {
			for _, page := range vmssvmResp {
				resp.AddPage(http.StatusOK, page, nil)
			}
			return
		},
	}
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
						ID: to.Ptr(defaultMockNetworkID),
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
					{ID: to.Ptr(defaultMockNetworkID)},
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

func sortTargetsByID(targets []model.LabelSet) {
	slices.SortFunc(targets, func(a, b model.LabelSet) int {
		return strings.Compare(string(a["__meta_azure_machine_id"]), string(b["__meta_azure_machine_id"]))
	})
}
