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
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2018-10-01/compute"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestMapFromVMWithEmptyTags(t *testing.T) {
	id := "test"
	name := "name"
	vmType := "type"
	location := "westeurope"
	networkProfile := compute.NetworkProfile{
		NetworkInterfaces: &[]compute.NetworkInterfaceReference{},
	}
	properties := &compute.VirtualMachineProperties{
		StorageProfile: &compute.StorageProfile{
			OsDisk: &compute.OSDisk{
				OsType: "Linux",
			},
		},
		NetworkProfile: &networkProfile,
	}

	testVM := compute.VirtualMachine{
		ID:                       &id,
		Name:                     &name,
		Type:                     &vmType,
		Location:                 &location,
		Tags:                     nil,
		VirtualMachineProperties: properties,
	}

	expectedVM := virtualMachine{
		ID:                id,
		Name:              name,
		Type:              vmType,
		Location:          location,
		OsType:            "Linux",
		Tags:              map[string]*string{},
		NetworkInterfaces: []string{},
	}

	actualVM := mapFromVM(testVM)

	require.Equal(t, expectedVM, actualVM)
}

func TestMapFromVMWithTags(t *testing.T) {
	id := "test"
	name := "name"
	vmType := "type"
	location := "westeurope"
	tags := map[string]*string{
		"prometheus": new(string),
	}
	networkProfile := compute.NetworkProfile{
		NetworkInterfaces: &[]compute.NetworkInterfaceReference{},
	}
	properties := &compute.VirtualMachineProperties{
		StorageProfile: &compute.StorageProfile{
			OsDisk: &compute.OSDisk{
				OsType: "Linux",
			},
		},
		NetworkProfile: &networkProfile,
	}

	testVM := compute.VirtualMachine{
		ID:                       &id,
		Name:                     &name,
		Type:                     &vmType,
		Location:                 &location,
		Tags:                     tags,
		VirtualMachineProperties: properties,
	}

	expectedVM := virtualMachine{
		ID:                id,
		Name:              name,
		Type:              vmType,
		Location:          location,
		OsType:            "Linux",
		Tags:              tags,
		NetworkInterfaces: []string{},
	}

	actualVM := mapFromVM(testVM)

	require.Equal(t, expectedVM, actualVM)
}

func TestMapFromVMScaleSetVMWithEmptyTags(t *testing.T) {
	id := "test"
	name := "name"
	vmType := "type"
	location := "westeurope"
	networkProfile := compute.NetworkProfile{
		NetworkInterfaces: &[]compute.NetworkInterfaceReference{},
	}
	properties := &compute.VirtualMachineScaleSetVMProperties{
		StorageProfile: &compute.StorageProfile{
			OsDisk: &compute.OSDisk{
				OsType: "Linux",
			},
		},
		NetworkProfile: &networkProfile,
	}

	testVM := compute.VirtualMachineScaleSetVM{
		ID:                                 &id,
		Name:                               &name,
		Type:                               &vmType,
		Location:                           &location,
		Tags:                               nil,
		VirtualMachineScaleSetVMProperties: properties,
	}

	scaleSet := "testSet"
	expectedVM := virtualMachine{
		ID:                id,
		Name:              name,
		Type:              vmType,
		Location:          location,
		OsType:            "Linux",
		Tags:              map[string]*string{},
		NetworkInterfaces: []string{},
		ScaleSet:          scaleSet,
	}

	actualVM := mapFromVMScaleSetVM(testVM, scaleSet)

	require.Equal(t, expectedVM, actualVM)
}

func TestMapFromVMScaleSetVMWithTags(t *testing.T) {
	id := "test"
	name := "name"
	vmType := "type"
	location := "westeurope"
	tags := map[string]*string{
		"prometheus": new(string),
	}
	networkProfile := compute.NetworkProfile{
		NetworkInterfaces: &[]compute.NetworkInterfaceReference{},
	}
	properties := &compute.VirtualMachineScaleSetVMProperties{
		StorageProfile: &compute.StorageProfile{
			OsDisk: &compute.OSDisk{
				OsType: "Linux",
			},
		},
		NetworkProfile: &networkProfile,
	}

	testVM := compute.VirtualMachineScaleSetVM{
		ID:                                 &id,
		Name:                               &name,
		Type:                               &vmType,
		Location:                           &location,
		Tags:                               tags,
		VirtualMachineScaleSetVMProperties: properties,
	}

	scaleSet := "testSet"
	expectedVM := virtualMachine{
		ID:                id,
		Name:              name,
		Type:              vmType,
		Location:          location,
		OsType:            "Linux",
		Tags:              tags,
		NetworkInterfaces: []string{},
		ScaleSet:          scaleSet,
	}

	actualVM := mapFromVMScaleSetVM(testVM, scaleSet)

	require.Equal(t, expectedVM, actualVM)
}

func TestNewAzureResourceFromID(t *testing.T) {
	for _, tc := range []struct {
		id       string
		expected azureResource
	}{
		{
			id:       "/a/b/c/group/d/e/f/name",
			expected: azureResource{"name", "group"},
		},
		{
			id:       "/a/b/c/group/d/e/f/name/g/h",
			expected: azureResource{"name", "group"},
		},
	} {
		actual, _ := newAzureResourceFromID(tc.id, nil)
		require.Equal(t, tc.expected, actual)
	}
}
