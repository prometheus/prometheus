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

package outscale

import (
	"testing"

	osc "github.com/outscale/osc-sdk-go/v2"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func strptr(s string) *string { return &s }

func TestVmsToLabelSets(t *testing.T) {
	cfg := &SDConfig{
		Region: "eu-west-2",
		Port:   9090,
	}

	t.Run("empty", func(t *testing.T) {
		out := vmsToLabelSets(nil, cfg)
		require.Empty(t, out)
		out = vmsToLabelSets([]osc.Vm{}, cfg)
		require.Empty(t, out)
	})

	t.Run("skip_no_ip", func(t *testing.T) {
		vms := []osc.Vm{
			{VmId: strptr("i-1"), State: strptr("running")}, // no PrivateIp nor PublicIp
		}
		out := vmsToLabelSets(vms, cfg)
		require.Empty(t, out)
	})

	t.Run("private_ip_preferred", func(t *testing.T) {
		vms := []osc.Vm{
			{
				VmId:      strptr("i-abc"),
				State:     strptr("running"),
				PrivateIp: strptr("10.1.2.3"),
				PublicIp:  strptr("1.2.3.4"),
			},
		}
		out := vmsToLabelSets(vms, cfg)
		require.Len(t, out, 1)
		require.Equal(t, model.LabelValue("10.1.2.3:9090"), out[0][model.AddressLabel])
		require.Equal(t, model.LabelValue("i-abc"), out[0][vmLabelInstanceID])
		require.Equal(t, model.LabelValue("eu-west-2"), out[0][vmLabelRegion])
		require.Equal(t, model.LabelValue("running"), out[0][vmLabelState])
		require.Equal(t, model.LabelValue("10.1.2.3"), out[0][vmLabelPrivateIP])
		require.Equal(t, model.LabelValue("1.2.3.4"), out[0][vmLabelPublicIP])
	})

	t.Run("public_ip_fallback", func(t *testing.T) {
		vms := []osc.Vm{
			{
				VmId:     strptr("i-pub"),
				State:    strptr("running"),
				PublicIp: strptr("203.0.113.10"),
			},
		}
		out := vmsToLabelSets(vms, cfg)
		require.Len(t, out, 1)
		require.Equal(t, model.LabelValue("203.0.113.10:9090"), out[0][model.AddressLabel])
		require.Equal(t, model.LabelValue("203.0.113.10"), out[0][vmLabelPublicIP])
		_, hasPrivate := out[0][vmLabelPrivateIP]
		require.False(t, hasPrivate)
	})

	t.Run("subregion_and_tags", func(t *testing.T) {
		subregion := "eu-west-2a"
		vms := []osc.Vm{
			{
				VmId:      strptr("i-tagged"),
				State:     strptr("stopped"),
				PrivateIp: strptr("10.0.0.5"),
				Placement: &osc.Placement{SubregionName: &subregion},
				Tags: &[]osc.ResourceTag{
					{Key: "Name", Value: "my-vm"},
					{Key: "env", Value: "prod"},
				},
			},
		}
		out := vmsToLabelSets(vms, cfg)
		require.Len(t, out, 1)
		require.Equal(t, model.LabelValue("eu-west-2a"), out[0][vmLabelSubregion])
		require.Equal(t, model.LabelValue("my-vm"), out[0][vmLabelTag+model.LabelName("Name")])
		require.Equal(t, model.LabelValue("prod"), out[0][vmLabelTag+model.LabelName("env")])
	})

	t.Run("skips_empty_tag_key_or_value", func(t *testing.T) {
		vms := []osc.Vm{
			{
				VmId:      strptr("i-tags"),
				PrivateIp: strptr("10.0.0.1"),
				Tags: &[]osc.ResourceTag{
					{Key: "Good", Value: "yes"},
					{Key: "", Value: "skip"},
					{Key: "EmptyVal", Value: ""},
				},
			},
		}
		out := vmsToLabelSets(vms, cfg)
		require.Len(t, out, 1)
		require.Equal(t, model.LabelValue("yes"), out[0][vmLabelTag+model.LabelName("Good")])
		_, hasBad := out[0][vmLabelTag+model.LabelName("")]
		require.False(t, hasBad)
		_, hasEmptyVal := out[0][vmLabelTag+model.LabelName("EmptyVal")]
		require.False(t, hasEmptyVal)
	})
}
