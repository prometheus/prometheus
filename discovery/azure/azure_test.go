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
	"github.com/prometheus/common/promslog"
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
		logger: promslog.NewNopLogger(),
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

func TestAzureSDRefresh(t *testing.T) {
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

func createNewMockAzureClient(logger log.Logger) (client, error) {
	networkID := "/subscriptions/00000000-0000-0000-0000-000000000000/network1"
	ipAddress := "10.20.30.40"
	primary := true

	c := mockAzureClient{}

	// Create mock servers.
	mockVMServer := newDefaultMockVMServer()
	mockVMSSServer := newDefaultMockVMSSServer()
	mockVMScaleSetVMServer := newDefaultMockVMSSVMServer()

	// Connect our client with the mock servers using mock transports (round-trippers).
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
		Type:     to.Ptr("type"),
		Location: to.Ptr("westeurope"),
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
				NetworkInterfaces: []*armcompute.NetworkInterfaceReference{},
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

func newDefaultMockVMServer() fake.VirtualMachinesServer {

	fakeVirtualMachinesServer := fake.VirtualMachinesServer{

		NewListAllPager: func(options *armcompute.VirtualMachinesClientListAllOptions) (resp azfake.PagerResponder[armcompute.VirtualMachinesClientListAllResponse]) {

			page1 := armcompute.VirtualMachinesClientListAllResponse{
				VirtualMachineListResult: armcompute.VirtualMachineListResult{
					Value: []*armcompute.VirtualMachine{
						defaultVMWithIdAndName(to.Ptr("/fake/resource/id-1"), to.Ptr("fake-vm-1")),
						defaultVMWithIdAndName(to.Ptr("/fake/resource/id-2"), to.Ptr("fake-vm-2")),
					},
				},
			}

			page2 := armcompute.VirtualMachinesClientListAllResponse{
				VirtualMachineListResult: armcompute.VirtualMachineListResult{
					Value: []*armcompute.VirtualMachine{
						defaultVMWithIdAndName(to.Ptr("/fake/resource/id-3"), to.Ptr("fake-vm-3")),
						defaultVMWithIdAndName(to.Ptr("/fake/resource/id-4"), to.Ptr("fake-vm-4")),
					},
				},
			}

			resp.AddPage(http.StatusOK, page1, nil)
			resp.AddPage(http.StatusOK, page2, nil)

			return
		},
	}
	return fakeVirtualMachinesServer
}

func newDefaultMockVMSSServer() fake.VirtualMachineScaleSetsServer {
	fakeVirtualMachineScaleSetsServer := fake.VirtualMachineScaleSetsServer{

		NewListAllPager: func(options *armcompute.VirtualMachineScaleSetsClientListAllOptions) (resp azfake.PagerResponder[armcompute.VirtualMachineScaleSetsClientListAllResponse]) {

			page1 := armcompute.VirtualMachineScaleSetsClientListAllResponse{
				VirtualMachineScaleSetListWithLinkResult: armcompute.VirtualMachineScaleSetListWithLinkResult{
					Value: []*armcompute.VirtualMachineScaleSet{
						{
							ID:   to.Ptr("/fake/scaleset/id-1"),
							Name: to.Ptr("fake-scaleset-1"),
						},
						{
							ID:   to.Ptr("/fake/scaleset/id-2"),
							Name: to.Ptr("fake-scaleset-2"),
						},
					},
				},
			}

			page2 := armcompute.VirtualMachineScaleSetsClientListAllResponse{
				VirtualMachineScaleSetListWithLinkResult: armcompute.VirtualMachineScaleSetListWithLinkResult{
					Value: []*armcompute.VirtualMachineScaleSet{
						{
							ID:   to.Ptr("/fake/scaleset/id-3"),
							Name: to.Ptr("fake-scaleset-3"),
						},
						{
							ID:   to.Ptr("/fake/scaleset/id-4"),
							Name: to.Ptr("fake-scaleset-4"),
						},
					},
				},
			}

			resp.AddPage(http.StatusOK, page1, nil)
			resp.AddPage(http.StatusOK, page2, nil)

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
		Type:       to.Ptr("type"),
		Location:   to.Ptr("westeurope"),
		InstanceID: to.Ptr("123"),
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
				NetworkInterfaces: []*armcompute.NetworkInterfaceReference{},
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

func newDefaultMockVMSSVMServer() fake.VirtualMachineScaleSetVMsServer {

	fakeVirtualMachineScaleSetVMsServer := fake.VirtualMachineScaleSetVMsServer{

		NewListPager: func(resourceGroupName string, virtualMachineScaleSetName string, options *armcompute.VirtualMachineScaleSetVMsClientListOptions) (resp azfake.PagerResponder[armcompute.VirtualMachineScaleSetVMsClientListResponse]) {

			page1 := armcompute.VirtualMachineScaleSetVMsClientListResponse{
				VirtualMachineScaleSetVMListResult: armcompute.VirtualMachineScaleSetVMListResult{
					Value: []*armcompute.VirtualMachineScaleSetVM{
						defaultVMSSVMWithIdAndName(to.Ptr("/fake/scalesetvm/id-1"), to.Ptr("fake-scalesetvm-1")),
						defaultVMSSVMWithIdAndName(to.Ptr("/fake/scalesetvm/id-2"), to.Ptr("fake-scalesetvm-2")),
					},
				},
			}

			page2 := armcompute.VirtualMachineScaleSetVMsClientListResponse{
				VirtualMachineScaleSetVMListResult: armcompute.VirtualMachineScaleSetVMListResult{
					Value: []*armcompute.VirtualMachineScaleSetVM{
						defaultVMSSVMWithIdAndName(to.Ptr("/fake/scalesetvm/id-3"), to.Ptr("fake-scalesetvm-3")),
						defaultVMSSVMWithIdAndName(to.Ptr("/fake/scalesetvm/id-4"), to.Ptr("fake-scalesetvm-4")),
					},
				},
			}

			resp.AddPage(http.StatusOK, page1, nil)
			resp.AddPage(http.StatusOK, page2, nil)

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
