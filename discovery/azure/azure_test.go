// Copyright The Prometheus Authors
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
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v8"
	fake "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v8/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v8"
	fakenetwork "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v8/fake"
	cache "github.com/Code-Hex/go-generics-cache"
	"github.com/Code-Hex/go-generics-cache/policy/lru"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
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
	require.Len(t, labelSet, 12)
}

func TestVMToLabelSetWithNilTagValue(t *testing.T) {
	// A tag with a JSON null value, as returned by the ARM API, unmarshals to a
	// nil pointer that is still present in the Tags map.
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
		ID:       &id,
		Name:     &name,
		Type:     &vmType,
		Location: &location,
		Tags: map[string]*string{
			"CostCenter": nil,
			"env":        to.Ptr("prod"),
		},
		Properties: properties,
	}

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

	labelSet, err := d.vmToLabelSet(context.Background(), client, mapFromVM(testVM))
	require.NoError(t, err)
	// The tag with a value is kept, the one with a nil value is skipped.
	require.Equal(t, model.LabelValue("prod"), labelSet[azureLabelMachineTag+"env"])
	require.NotContains(t, labelSet, model.LabelName(azureLabelMachineTag+"CostCenter"))
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
		InstanceView: &armcompute.VirtualMachineScaleSetVMInstanceView{
			Statuses: []*armcompute.InstanceViewStatus{
				{
					Code: to.Ptr("PowerState/running"),
				},
			},
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
		PowerState:        "running",
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
		InstanceView: &armcompute.VirtualMachineScaleSetVMInstanceView{
			Statuses: []*armcompute.InstanceViewStatus{
				{
					Code: to.Ptr("PowerState/running"),
				},
			},
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
		PowerState:        "running",
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
		InstanceView: &armcompute.VirtualMachineScaleSetVMInstanceView{
			Statuses: []*armcompute.InstanceViewStatus{
				{
					Code: to.Ptr("PowerState/running"),
				},
			},
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
		PowerState:        "running",
	}

	actualVM := mapFromVMScaleSetVM(testVM, scaleSet)

	require.Equal(t, expectedVM, actualVM)
}

func TestPowerStateFromStatuses(t *testing.T) {
	for _, tc := range []struct {
		name     string
		statuses []*armcompute.InstanceViewStatus
		expected string
	}{
		{
			name: "running",
			statuses: []*armcompute.InstanceViewStatus{
				{Code: to.Ptr("ProvisioningState/succeeded")},
				{Code: to.Ptr("PowerState/running")},
			},
			expected: "running",
		},
		{
			name:     "stopped",
			statuses: []*armcompute.InstanceViewStatus{{Code: to.Ptr("PowerState/stopped")}},
			expected: "stopped",
		},
		{
			name:     "deallocated",
			statuses: []*armcompute.InstanceViewStatus{{Code: to.Ptr("PowerState/deallocated")}},
			expected: "deallocated",
		},
		{
			name:     "starting",
			statuses: []*armcompute.InstanceViewStatus{{Code: to.Ptr("PowerState/starting")}},
			expected: "starting",
		},
		{
			name:     "stopping",
			statuses: []*armcompute.InstanceViewStatus{{Code: to.Ptr("PowerState/stopping")}},
			expected: "stopping",
		},
		{
			name:     "deallocating",
			statuses: []*armcompute.InstanceViewStatus{{Code: to.Ptr("PowerState/deallocating")}},
			expected: "deallocating",
		},
		{
			name:     "no power state",
			statuses: []*armcompute.InstanceViewStatus{{Code: to.Ptr("ProvisioningState/succeeded")}, nil, {}},
			expected: "",
		},
		{
			name:     "no statuses",
			expected: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, powerStateFromStatuses(tc.statuses))
		})
	}
}

func TestGetVMPowerStates(t *testing.T) {
	vm1ID := "/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/RG1/providers/Microsoft.Compute/virtualMachines/vm1"
	vm2ID := "/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/RG1/providers/Microsoft.Compute/virtualMachines/vm2"
	vm1 := defaultVMWithIDAndName(&vm1ID, to.Ptr("vm1"))
	vm2 := defaultVMWithIDAndName(&vm2ID, to.Ptr("vm2"))
	vm2.Properties.InstanceView.Statuses = []*armcompute.InstanceViewStatus{{Code: to.Ptr("PowerState/deallocated")}}
	vmResp := []armcompute.VirtualMachinesClientListAllResponse{
		{VirtualMachineListResult: armcompute.VirtualMachineListResult{Value: []*armcompute.VirtualMachine{vm1}}},
		{VirtualMachineListResult: armcompute.VirtualMachineListResult{Value: []*armcompute.VirtualMachine{vm2}}},
	}

	mockVMServer := defaultMockVMServer(vmResp)
	vmClient, err := armcompute.NewVirtualMachinesClient("fake-subscription-id", &azfake.TokenCredential{}, &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: fake.NewVirtualMachinesServerTransport(&mockVMServer),
		},
	})
	require.NoError(t, err)
	client := &azureClient{vm: newVirtualMachinesClientAdapter(vmClient)}

	powerStates, err := client.getVMPowerStates(context.Background())
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		strings.ToLower(vm1ID): "running",
		strings.ToLower(vm2ID): "deallocated",
	}, powerStates)
}

// powerStatesClient is a client whose power states are set by the test.
type powerStatesClient struct {
	client
	powerStates map[string]string
	err         error
}

func (c *powerStatesClient) getVMPowerStates(context.Context) (map[string]string, error) {
	return c.powerStates, c.err
}

func TestVMPowerStatesLastKnown(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := DefaultSDConfig.NewDiscovererMetrics(reg, discovery.NewRefreshMetrics(reg))
	d, err := NewDiscovery(&DefaultSDConfig, discovery.DiscovererOptions{Metrics: metrics, SetName: "azure"})
	require.NoError(t, err)
	client := &powerStatesClient{}
	ctx := context.Background()

	// Nothing is known before the first successful listing.
	client.err = errors.New("throttled")
	require.Nil(t, d.vmPowerStates(ctx, client))

	client.powerStates, client.err = map[string]string{"vm1": "running"}, nil
	require.Equal(t, map[string]string{"vm1": "running"}, d.vmPowerStates(ctx, client))

	// A failed listing returns the last known power states.
	client.powerStates, client.err = nil, errors.New("throttled")
	require.Equal(t, map[string]string{"vm1": "running"}, d.vmPowerStates(ctx, client))

	client.powerStates, client.err = map[string]string{"vm1": "deallocated"}, nil
	require.Equal(t, map[string]string{"vm1": "deallocated"}, d.vmPowerStates(ctx, client))

	require.Equal(t, 2.0, promtestutil.ToFloat64(d.metrics.failuresCount))
}

func TestAzureRefreshPowerStatesUnavailable(t *testing.T) {
	vmResp := []armcompute.VirtualMachinesClientListAllResponse{
		{VirtualMachineListResult: armcompute.VirtualMachineListResult{Value: []*armcompute.VirtualMachine{defaultVMWithIDAndName(nil, nil)}}},
	}
	interfacesResp := armnetwork.Interface{
		ID: to.Ptr(defaultMockNetworkID),
		Properties: &armnetwork.InterfacePropertiesFormat{
			Primary: to.Ptr(true),
			IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
				{Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{PrivateIPAddress: to.Ptr("10.0.0.1")}},
			},
		},
	}
	client := &powerStatesClient{
		client: createMockAzureClient(t, vmResp, []armcompute.VirtualMachineScaleSetsClientListAllResponse{{}}, nil, interfacesResp, nil),
		err:    errors.New("forbidden"),
	}
	reg := prometheus.NewRegistry()
	metrics := DefaultSDConfig.NewDiscovererMetrics(reg, discovery.NewRefreshMetrics(reg))
	d, err := NewDiscovery(&DefaultSDConfig, discovery.DiscovererOptions{Metrics: metrics, SetName: "azure"})
	require.NoError(t, err)

	// Discovery still succeeds, without power states.
	tgs, err := d.refreshAzureClient(context.Background(), client)
	require.NoError(t, err)
	require.Len(t, tgs, 1)
	require.Len(t, tgs[0].Targets, 1)
	require.Equal(t, model.LabelValue(""), tgs[0].Targets[0][azureLabelMachineState])
}

func TestGetScaleSetVMsPowerState(t *testing.T) {
	scaleSetID := "/subscriptions/00000000-0000-0000-0000-00000000000/resourceGroups/rg1/providers/Microsoft.Compute/virtualMachineScaleSets/vmScaleSet1"
	vmssvmResp := []armcompute.VirtualMachineScaleSetVMsClientListResponse{
		{
			VirtualMachineScaleSetVMListResult: armcompute.VirtualMachineScaleSetVMListResult{
				Value: []*armcompute.VirtualMachineScaleSetVM{
					defaultVMSSVMWithIDAndName(to.Ptr(scaleSetID+"/virtualMachines/0"), to.Ptr("vmScaleSet1_0")),
				},
			},
		},
	}

	for _, tc := range []struct {
		name             string
		orchestration    *armcompute.OrchestrationMode
		expectedState    string
		expectExpandUsed bool
	}{
		{
			name:             "orchestration mode not set",
			expectedState:    "running",
			expectExpandUsed: true,
		},
		{
			name:             "uniform",
			orchestration:    to.Ptr(armcompute.OrchestrationModeUniform),
			expectedState:    "running",
			expectExpandUsed: true,
		},
		{
			name:          "flexible",
			orchestration: to.Ptr(armcompute.OrchestrationModeFlexible),
			expectedState: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expandUsed := false
			mockServer := defaultMockVMSSVMServer(vmssvmResp)
			listPager := mockServer.NewListPager
			mockServer.NewListPager = func(resourceGroupName, scaleSetName string, options *armcompute.VirtualMachineScaleSetVMsClientListOptions) (resp azfake.PagerResponder[armcompute.VirtualMachineScaleSetVMsClientListResponse]) {
				expandUsed = options != nil && options.Expand != nil
				// Azure rejects $expand=instanceView on flexible scale sets.
				if expandUsed && tc.orchestration != nil && *tc.orchestration == armcompute.OrchestrationModeFlexible {
					resp.AddError(&azcore.ResponseError{StatusCode: http.StatusBadRequest, ErrorCode: "BadRequest"})
					return resp
				}
				return listPager(resourceGroupName, scaleSetName, options)
			}
			vmssvmClient, err := armcompute.NewVirtualMachineScaleSetVMsClient("fake-subscription-id", &azfake.TokenCredential{}, &arm.ClientOptions{
				ClientOptions: azcore.ClientOptions{
					Transport: fake.NewVirtualMachineScaleSetVMsServerTransport(&mockServer),
				},
			})
			require.NoError(t, err)
			client := &azureClient{
				vmssvm: newVirtualMachineScaleSetVMsClientAdapter(vmssvmClient),
				logger: promslog.NewNopLogger(),
			}

			scaleSet := armcompute.VirtualMachineScaleSet{
				ID:   &scaleSetID,
				Name: to.Ptr("vmScaleSet1"),
			}
			if tc.orchestration != nil {
				scaleSet.Properties = &armcompute.VirtualMachineScaleSetProperties{OrchestrationMode: tc.orchestration}
			}

			vms, err := client.getScaleSetVMs(context.Background(), scaleSet)
			require.NoError(t, err)
			require.Equal(t, tc.expectExpandUsed, expandUsed)
			require.Len(t, vms, 1)
			require.Equal(t, tc.expectedState, vms[0].PowerState)
		})
	}
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

func TestNewCredentialManagedIdentity(t *testing.T) {
	// Test that system-assigned managed identity (empty ClientID) creates
	// a valid credential. Previously, an empty ClientID was passed as
	// azidentity.ClientID("") which is not nil and caused Azure SDK to
	// look up a non-existent user-assigned identity instead of falling
	// back to system-assigned identity.
	cfg := SDConfig{
		AuthenticationMethod: authMethodManagedIdentity,
		ClientID:             "",
	}
	cred, err := newCredential(cfg, policy.ClientOptions{})
	require.NoError(t, err)
	require.NotNil(t, cred)

	// Test that user-assigned managed identity (non-empty ClientID) also works.
	cfg.ClientID = "00000000-0000-0000-0000-000000000000"
	cred, err = newCredential(cfg, policy.ClientOptions{})
	require.NoError(t, err)
	require.NotNil(t, cred)
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
							"__meta_azure_machine_state":          "running",
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
							"__meta_azure_machine_state":          "running",
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
							"__meta_azure_machine_state":          "running",
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
							"__meta_azure_machine_state":          "running",
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
							"__meta_azure_machine_state":          "running",
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
							"__meta_azure_machine_state":          "running",
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

			sd, err := NewDiscovery(azureSDConfig, discovery.DiscovererOptions{
				Logger:  nil,
				Metrics: metrics,
				SetName: "azure",
			})
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
	// statusClient serves getVMPowerStates. The fake server tracks pagers by URL path,
	// so concurrent list requests to the same path need their own fake server.
	statusClient azureClient
}

func (c *mockAzureClient) getVMPowerStates(ctx context.Context) (map[string]string, error) {
	return c.statusClient.getVMPowerStates(ctx)
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

	mockStatusVMServer := defaultMockVMServer(vmResp)
	statusVMClient, err := armcompute.NewVirtualMachinesClient("fake-subscription-id", &azfake.TokenCredential{}, &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: fake.NewVirtualMachinesServerTransport(&mockStatusVMServer),
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
			vm:     newVirtualMachinesClientAdapter(vmClient),
			vmss:   newVirtualMachineScaleSetsClientAdapter(vmssClient),
			vmssvm: newVirtualMachineScaleSetVMsClientAdapter(vmssvmClient),
			nic:    newInterfacesClientAdapter(interfacesClient),
			logger: logger,
		},
		statusClient: azureClient{
			vm:     newVirtualMachinesClientAdapter(statusVMClient),
			logger: logger,
		},
	}
}

func defaultMockInterfaceServer(interfaceResp armnetwork.Interface) fakenetwork.InterfacesServer {
	return fakenetwork.InterfacesServer{
		Get: func(context.Context, string, string, *armnetwork.InterfacesClientGetOptions) (resp azfake.Responder[armnetwork.InterfacesClientGetResponse], errResp azfake.ErrorResponder) {
			resp.SetResponse(http.StatusOK, armnetwork.InterfacesClientGetResponse{Interface: interfaceResp}, nil)
			return resp, errResp
		},
		GetVirtualMachineScaleSetNetworkInterface: func(context.Context, string, string, string, string, *armnetwork.InterfacesClientGetVirtualMachineScaleSetNetworkInterfaceOptions) (resp azfake.Responder[armnetwork.InterfacesClientGetVirtualMachineScaleSetNetworkInterfaceResponse], errResp azfake.ErrorResponder) {
			resp.SetResponse(http.StatusOK, armnetwork.InterfacesClientGetVirtualMachineScaleSetNetworkInterfaceResponse{Interface: interfaceResp}, nil)
			return resp, errResp
		},
	}
}

// defaultMockVMServer mimics the Azure API: the instance view of the given VMs is only
// returned with statusOnly, and $expand=instanceView is rejected without a scale set filter.
func defaultMockVMServer(vmResp []armcompute.VirtualMachinesClientListAllResponse) fake.VirtualMachinesServer {
	listVMs := func(statusOnly bool) []armcompute.VirtualMachineListResult {
		var pages []armcompute.VirtualMachineListResult
		for _, page := range vmResp {
			var vms []*armcompute.VirtualMachine
			for _, vm := range page.Value {
				if statusOnly {
					vms = append(vms, &armcompute.VirtualMachine{
						ID:         vm.ID,
						Name:       vm.Name,
						Properties: &armcompute.VirtualMachineProperties{InstanceView: vm.Properties.InstanceView},
					})
					continue
				}
				vmCopy := *vm
				propertiesCopy := *vm.Properties
				propertiesCopy.InstanceView = nil
				vmCopy.Properties = &propertiesCopy
				vms = append(vms, &vmCopy)
			}
			pages = append(pages, armcompute.VirtualMachineListResult{Value: vms})
		}
		return pages
	}
	expandErr := &azcore.ResponseError{StatusCode: http.StatusBadRequest, ErrorCode: "BadRequest"}

	return fake.VirtualMachinesServer{
		NewListAllPager: func(options *armcompute.VirtualMachinesClientListAllOptions) (resp azfake.PagerResponder[armcompute.VirtualMachinesClientListAllResponse]) {
			if options != nil && options.Expand != nil && options.Filter == nil {
				resp.AddError(expandErr)
				return resp
			}
			statusOnly := options != nil && options.StatusOnly != nil && *options.StatusOnly == "true"
			for _, page := range listVMs(statusOnly) {
				resp.AddPage(http.StatusOK, armcompute.VirtualMachinesClientListAllResponse{VirtualMachineListResult: page}, nil)
			}
			return resp
		},
		NewListPager: func(_ string, options *armcompute.VirtualMachinesClientListOptions) (resp azfake.PagerResponder[armcompute.VirtualMachinesClientListResponse]) {
			if options != nil && options.Expand != nil && options.Filter == nil {
				resp.AddError(expandErr)
				return resp
			}
			for _, page := range listVMs(false) {
				resp.AddPage(http.StatusOK, armcompute.VirtualMachinesClientListResponse{VirtualMachineListResult: page}, nil)
			}
			return resp
		},
	}
}

func defaultMockVMSSServer(vmssResp []armcompute.VirtualMachineScaleSetsClientListAllResponse) fake.VirtualMachineScaleSetsServer {
	return fake.VirtualMachineScaleSetsServer{
		NewListAllPager: func(*armcompute.VirtualMachineScaleSetsClientListAllOptions) (resp azfake.PagerResponder[armcompute.VirtualMachineScaleSetsClientListAllResponse]) {
			for _, page := range vmssResp {
				resp.AddPage(http.StatusOK, page, nil)
			}
			return resp
		},
	}
}

func defaultMockVMSSVMServer(vmssvmResp []armcompute.VirtualMachineScaleSetVMsClientListResponse) fake.VirtualMachineScaleSetVMsServer {
	return fake.VirtualMachineScaleSetVMsServer{
		NewListPager: func(_, _ string, options *armcompute.VirtualMachineScaleSetVMsClientListOptions) (resp azfake.PagerResponder[armcompute.VirtualMachineScaleSetVMsClientListResponse]) {
			expandInstanceView := options != nil && options.Expand != nil && *options.Expand == "instanceView"
			for _, page := range vmssvmResp {
				if !expandInstanceView {
					var vms []*armcompute.VirtualMachineScaleSetVM
					for _, vm := range page.Value {
						vmCopy := *vm
						propertiesCopy := *vm.Properties
						propertiesCopy.InstanceView = nil
						vmCopy.Properties = &propertiesCopy
						vms = append(vms, &vmCopy)
					}
					page.Value = vms
				}
				resp.AddPage(http.StatusOK, page, nil)
			}
			return resp
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
			InstanceView: &armcompute.VirtualMachineInstanceView{
				Statuses: []*armcompute.InstanceViewStatus{
					{
						Code: to.Ptr("PowerState/running"),
					},
				},
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
			InstanceView: &armcompute.VirtualMachineScaleSetVMInstanceView{
				Statuses: []*armcompute.InstanceViewStatus{
					{
						Code: to.Ptr("PowerState/running"),
					},
				},
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
