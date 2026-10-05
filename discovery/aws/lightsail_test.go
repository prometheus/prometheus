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

package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	"github.com/aws/aws-sdk-go-v2/service/lightsail/types"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

// newMockLightsailClient returns an adapter whose GetInstances answers with the
// supplied instances, so refresh() can be exercised without reaching AWS.
func newMockLightsailClient(instances []types.Instance) *lightsailClientAdapter {
	return &lightsailClientAdapter{
		getInstances: func(_ context.Context, _ *lightsail.GetInstancesInput, _ ...func(*lightsail.Options)) (*lightsail.GetInstancesOutput, error) {
			return &lightsail.GetInstancesOutput{Instances: instances}, nil
		},
	}
}

func lightsailTestDiscovery(instances []types.Instance) *LightsailDiscovery {
	return &LightsailDiscovery{
		lightsail: newMockLightsailClient(instances),
		cfg:       &LightsailSDConfig{Port: 8080},
		region:    "us-east-1",
	}
}

// fullyPopulatedLightsailInstance is the shape the AWS API returns when every
// optional field happens to be set. Each subtest below blanks exactly one of
// them.
func fullyPopulatedLightsailInstance() types.Instance {
	return types.Instance{
		PrivateIpAddress: strptr("10.0.0.1"),
		BlueprintId:      strptr("ubuntu_22_04"),
		BundleId:         strptr("nano_2_0"),
		Name:             strptr("instance-1"),
		SupportCode:      strptr("support-code-1"),
		Location:         &types.ResourceLocation{AvailabilityZone: strptr("us-east-1a")},
		State:            &types.InstanceState{Name: strptr("running")},
	}
}

// TestLightsailRefreshFullyPopulated pins the happy path so the nil-handling
// added for the cases below cannot silently drop labels that used to be set.
func TestLightsailRefreshFullyPopulated(t *testing.T) {
	t.Parallel()

	tgs, err := lightsailTestDiscovery([]types.Instance{fullyPopulatedLightsailInstance()}).refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs, 1)
	require.Len(t, tgs[0].Targets, 1)

	target := tgs[0].Targets[0]
	require.Equal(t, model.LabelValue("10.0.0.1:8080"), target[model.AddressLabel])
	require.Equal(t, model.LabelValue("us-east-1a"), target[lightsailLabelAZ])
	require.Equal(t, model.LabelValue("ubuntu_22_04"), target[lightsailLabelBlueprintID])
	require.Equal(t, model.LabelValue("nano_2_0"), target[lightsailLabelBundleID])
	require.Equal(t, model.LabelValue("instance-1"), target[lightsailLabelInstanceName])
	require.Equal(t, model.LabelValue("running"), target[lightsailLabelInstanceState])
	require.Equal(t, model.LabelValue("support-code-1"), target[lightsailLabelInstanceSupportCode])
}

// TestLightsailRefreshNilOptionalFields covers instances where the AWS API
// omitted an optional field. Every field blanked here is a pointer in the SDK
// (Location and State are pointers to structs whose members are themselves
// pointers), and refresh() dereferenced all of them without a nil check, so a
// single missing field panicked the whole Prometheus process during service
// discovery rather than degrading the target.
func TestLightsailRefreshNilOptionalFields(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		mutate  func(*types.Instance)
		absent  model.LabelName
		present map[model.LabelName]model.LabelValue
	}{
		{
			name:   "NilLocation",
			mutate: func(i *types.Instance) { i.Location = nil },
			absent: lightsailLabelAZ,
		},
		{
			name:   "NilLocationAvailabilityZone",
			mutate: func(i *types.Instance) { i.Location = &types.ResourceLocation{} },
			absent: lightsailLabelAZ,
		},
		{
			name:   "NilState",
			mutate: func(i *types.Instance) { i.State = nil },
			absent: lightsailLabelInstanceState,
		},
		{
			name:   "NilStateName",
			mutate: func(i *types.Instance) { i.State = &types.InstanceState{} },
			absent: lightsailLabelInstanceState,
		},
		{
			name:   "NilBlueprintId",
			mutate: func(i *types.Instance) { i.BlueprintId = nil },
			absent: lightsailLabelBlueprintID,
		},
		{
			name:   "NilBundleId",
			mutate: func(i *types.Instance) { i.BundleId = nil },
			absent: lightsailLabelBundleID,
		},
		{
			name:   "NilName",
			mutate: func(i *types.Instance) { i.Name = nil },
			absent: lightsailLabelInstanceName,
		},
		{
			name:   "NilSupportCode",
			mutate: func(i *types.Instance) { i.SupportCode = nil },
			absent: lightsailLabelInstanceSupportCode,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			inst := fullyPopulatedLightsailInstance()
			tc.mutate(&inst)

			tgs, err := lightsailTestDiscovery([]types.Instance{inst}).refresh(context.Background())
			require.NoError(t, err)
			require.Len(t, tgs, 1)
			require.Len(t, tgs[0].Targets, 1,
				"instance must still be discovered when an optional field is absent")

			target := tgs[0].Targets[0]
			require.NotContains(t, target, tc.absent,
				"label sourced from the absent field should be omitted")
			// The instance is still usable: the address is what makes it a target.
			require.Equal(t, model.LabelValue("10.0.0.1:8080"), target[model.AddressLabel])
		})
	}
}

// TestLightsailRefreshAllOptionalFieldsNil is the serverless-ish worst case:
// only the private IP is present. It must still yield a scrapeable target.
func TestLightsailRefreshAllOptionalFieldsNil(t *testing.T) {
	t.Parallel()

	tgs, err := lightsailTestDiscovery([]types.Instance{
		{PrivateIpAddress: strptr("10.0.0.9")},
	}).refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs, 1)
	require.Len(t, tgs[0].Targets, 1)

	target := tgs[0].Targets[0]
	require.Equal(t, model.LabelValue("10.0.0.9:8080"), target[model.AddressLabel])
	require.Equal(t, model.LabelValue("us-east-1"), target[lightsailLabelRegion])
	for _, absent := range []model.LabelName{
		lightsailLabelAZ,
		lightsailLabelBlueprintID,
		lightsailLabelBundleID,
		lightsailLabelInstanceName,
		lightsailLabelInstanceState,
		lightsailLabelInstanceSupportCode,
	} {
		require.NotContains(t, target, absent)
	}
}
