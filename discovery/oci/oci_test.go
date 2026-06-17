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

package oci

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

// ---- test helpers -----------------------------------------------------------

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

// makeInstance builds a core.Instance with sensible defaults.
func makeInstance(opts ...func(*core.Instance)) core.Instance {
	inst := core.Instance{
		Id:                 strPtr("ocid1.instance.oc1.iad.aaa001"),
		DisplayName:        strPtr("test-instance-1"),
		Shape:              strPtr("VM.Standard.E4.Flex"),
		AvailabilityDomain: strPtr("US-ASHBURN-AD-1"),
		FaultDomain:        strPtr("FAULT-DOMAIN-1"),
		ImageId:            strPtr("ocid1.image.oc1.iad.img001"),
		Region:             strPtr("us-ashburn-1"),
		CompartmentId:      strPtr("ocid1.compartment.oc1..comp001"),
		LifecycleState:     core.InstanceLifecycleStateRunning,
		FreeformTags:       map[string]string{},
		DefinedTags:        map[string]map[string]any{},
	}
	for _, o := range opts {
		o(&inst)
	}
	return inst
}

// makeVnic builds a primary VNIC. publicIP may be empty.
func makeVnic(id, privateIP, publicIP string) *core.Vnic {
	v := &core.Vnic{
		Id:        strPtr(id),
		IsPrimary: boolPtr(true),
		PrivateIp: strPtr(privateIP),
	}
	if publicIP != "" {
		v.PublicIp = strPtr(publicIP)
	}
	return v
}

// makeAttachment returns a single ATTACHED VnicAttachment.
func makeAttachment(instanceID, vnicID string) core.VnicAttachment {
	return core.VnicAttachment{
		Id:             strPtr("ocid1.vnicattachment.oc1..att001"),
		InstanceId:     strPtr(instanceID),
		VnicId:         strPtr(vnicID),
		LifecycleState: core.VnicAttachmentLifecycleStateAttached,
	}
}

const (
	testCompartment = "ocid1.compartment.oc1..comp001"
	testTenancy     = "ocid1.tenancy.oc1..test001"
)

// baseDiscovery returns a Discovery with no-op closure stubs.
// Tests override specific closures. d.refresh() is called directly so
// d.Discovery (the refresh framework) is left nil.
func baseDiscovery() *Discovery {
	return &Discovery{
		tenancy:         testTenancy,
		tagFilterAction: tagFilterActionInclude,
		port:            9100,
		logger:          slog.Default(),
		listCompartments: func(_ context.Context) ([]string, error) {
			return []string{testCompartment}, nil
		},
		listInstances:       func(_ context.Context, _ string) ([]core.Instance, error) { return nil, nil },
		listVnicAttachments: func(_ context.Context, _, _ string) ([]core.VnicAttachment, error) { return nil, nil },
		getVnic:             func(_ context.Context, _ string) (*core.Vnic, error) { return nil, nil },
	}
}

// singleInstanceDiscovery wires closures to serve exactly one instance/VNIC.
func singleInstanceDiscovery(inst core.Instance, vnic *core.Vnic) *Discovery {
	att := makeAttachment(stringVal(inst.Id), stringVal(vnic.Id))
	d := baseDiscovery()
	d.listInstances = func(_ context.Context, _ string) ([]core.Instance, error) {
		return []core.Instance{inst}, nil
	}
	d.listVnicAttachments = func(_ context.Context, _, _ string) ([]core.VnicAttachment, error) {
		return []core.VnicAttachment{att}, nil
	}
	d.getVnic = func(_ context.Context, _ string) (*core.Vnic, error) {
		return vnic, nil
	}
	return d
}

// ---- hasTag -----------------------------------------------------------------

func TestHasTag(t *testing.T) {
	cases := []struct {
		name  string
		inst  core.Instance
		key   string
		value string
		want  bool
	}{
		{
			name: "freeform match",
			inst: makeInstance(func(i *core.Instance) {
				i.FreeformTags = map[string]string{"monitoring": "enabled"}
			}),
			key: "monitoring", value: "enabled", want: true,
		},
		{
			name: "freeform value mismatch",
			inst: makeInstance(func(i *core.Instance) {
				i.FreeformTags = map[string]string{"monitoring": "disabled"}
			}),
			key: "monitoring", value: "enabled", want: false,
		},
		{
			name: "freeform key absent",
			inst: makeInstance(),
			key:  "monitoring", value: "enabled", want: false,
		},
		{
			name: "defined tag match",
			inst: makeInstance(func(i *core.Instance) {
				i.DefinedTags = map[string]map[string]any{
					"ops-ns": {"monitoring": "enabled"},
				}
			}),
			key: "monitoring", value: "enabled", want: true,
		},
		{
			name: "defined tag value mismatch",
			inst: makeInstance(func(i *core.Instance) {
				i.DefinedTags = map[string]map[string]any{
					"ops-ns": {"monitoring": "disabled"},
				}
			}),
			key: "monitoring", value: "enabled", want: false,
		},
		{
			name: "defined tag non-string value not matched",
			inst: makeInstance(func(i *core.Instance) {
				i.DefinedTags = map[string]map[string]any{
					"ops-ns": {"monitoring": 42},
				}
			}),
			key: "monitoring", value: "42", want: false,
		},
		{
			name: "defined tag found in second namespace",
			inst: makeInstance(func(i *core.Instance) {
				i.DefinedTags = map[string]map[string]any{
					"ns1": {"other": "x"},
					"ns2": {"monitoring": "enabled"},
				}
			}),
			key: "monitoring", value: "enabled", want: true,
		},
		{
			name: "freeform matches even when defined tag disagrees",
			inst: makeInstance(func(i *core.Instance) {
				i.FreeformTags = map[string]string{"monitoring": "enabled"}
				i.DefinedTags = map[string]map[string]any{
					"ns": {"monitoring": "disabled"},
				}
			}),
			key: "monitoring", value: "enabled", want: true,
		},
		{
			name: "nil freeform and nil defined - no panic",
			inst: makeInstance(func(i *core.Instance) {
				i.FreeformTags = nil
				i.DefinedTags = nil
			}),
			key: "monitoring", value: "enabled", want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, hasTag(&tc.inst, tc.key, tc.value))
		})
	}
}

// ---- instanceLabels ---------------------------------------------------------

func TestInstanceLabels_CoreLabels(t *testing.T) {
	inst := makeInstance()
	labels := instanceLabels(&inst, "10.0.0.5", "203.0.113.1", testTenancy, 9100)

	expected := model.LabelSet{
		model.AddressLabel:         "10.0.0.5:9100",
		ociLabelInstanceID:         "ocid1.instance.oc1.iad.aaa001",
		ociLabelInstanceName:       "test-instance-1",
		ociLabelInstanceState:      "RUNNING",
		ociLabelInstanceShape:      "VM.Standard.E4.Flex",
		ociLabelAvailabilityDomain: "US-ASHBURN-AD-1",
		ociLabelFaultDomain:        "FAULT-DOMAIN-1",
		ociLabelRegion:             "us-ashburn-1",
		ociLabelTenancyID:          testTenancy,
		ociLabelCompartmentID:      "ocid1.compartment.oc1..comp001",
		ociLabelImageID:            "ocid1.image.oc1.iad.img001",
		ociLabelPrivateIP:          "10.0.0.5",
		ociLabelPublicIP:           "203.0.113.1",
	}
	for k, want := range expected {
		require.Equal(t, want, labels[k], "label %s", k)
	}
}

func TestInstanceLabels_AddressIsPrivateIPWhenBothPresent(t *testing.T) {
	inst := makeInstance()
	labels := instanceLabels(&inst, "10.0.0.5", "203.0.113.1", "tenancy", 80)
	require.Equal(t, model.LabelValue("10.0.0.5:80"), labels[model.AddressLabel])
}

func TestInstanceLabels_AddressFallsBackToPublicIP(t *testing.T) {
	inst := makeInstance()
	labels := instanceLabels(&inst, "", "203.0.113.1", "tenancy", 80)
	require.Equal(t, model.LabelValue("203.0.113.1:80"), labels[model.AddressLabel])
}

func TestInstanceLabels_FreeformTagsSanitized(t *testing.T) {
	// Sanitization replaces invalid characters with underscores but preserves
	// case so that case-sensitive OCI tag keys do not collide (e.g. Env vs env).
	inst := makeInstance(func(i *core.Instance) {
		i.FreeformTags = map[string]string{
			"environment": "production",
			"My-Tag.Key":  "value",
			"123numeric":  "v",
			"Env":         "upper",
			"env":         "lower",
		}
	})
	labels := instanceLabels(&inst, "10.0.0.1", "", "tenancy", 80)

	require.Equal(t, model.LabelValue("production"), labels[ociLabelFreeformTagPrefix+"environment"])
	require.Equal(t, model.LabelValue("value"), labels[ociLabelFreeformTagPrefix+"My_Tag_Key"])
	require.Equal(t, model.LabelValue("v"), labels[ociLabelFreeformTagPrefix+"123numeric"])
	require.Equal(t, model.LabelValue("upper"), labels[ociLabelFreeformTagPrefix+"Env"])
	require.Equal(t, model.LabelValue("lower"), labels[ociLabelFreeformTagPrefix+"env"])
}

func TestInstanceLabels_DefinedTagsStringOnly(t *testing.T) {
	inst := makeInstance(func(i *core.Instance) {
		i.DefinedTags = map[string]map[string]any{
			"Oracle-Tags": {
				"CreatedBy":  "user@example.com",
				"CostCenter": "42",
				"IntValue":   100,  // must be skipped
				"BoolValue":  true, // must be skipped
			},
		}
	})
	labels := instanceLabels(&inst, "10.0.0.1", "", "tenancy", 80)

	require.Equal(t, model.LabelValue("user@example.com"), labels[ociLabelDefinedTagPrefix+"Oracle_Tags_CreatedBy"])
	require.Equal(t, model.LabelValue("42"), labels[ociLabelDefinedTagPrefix+"Oracle_Tags_CostCenter"])

	_, hasInt := labels[ociLabelDefinedTagPrefix+"Oracle_Tags_IntValue"]
	require.False(t, hasInt, "integer defined tag must not be emitted")

	_, hasBool := labels[ociLabelDefinedTagPrefix+"Oracle_Tags_BoolValue"]
	require.False(t, hasBool, "boolean defined tag must not be emitted")
}

func TestInstanceLabels_NilOptionalFields(t *testing.T) {
	inst := core.Instance{
		LifecycleState: core.InstanceLifecycleStateRunning,
		FreeformTags:   map[string]string{},
		DefinedTags:    map[string]map[string]any{},
	}
	labels := instanceLabels(&inst, "10.0.0.1", "", "tenancy", 80)

	require.Equal(t, model.LabelValue("10.0.0.1:80"), labels[model.AddressLabel])
	require.Equal(t, model.LabelValue(""), labels[ociLabelInstanceID])
	require.Equal(t, model.LabelValue(""), labels[ociLabelImageID])
}

func TestInstanceLabels_PortInAddress(t *testing.T) {
	inst := makeInstance()
	for _, port := range []int{80, 9100, 9182, 443} {
		labels := instanceLabels(&inst, "10.0.0.1", "", "tenancy", port)
		require.Equal(
			t,
			model.LabelValue("10.0.0.1:"+strconv.Itoa(port)),
			labels[model.AddressLabel],
			"port %d", port,
		)
	}
}

// ---- refresh ----------------------------------------------------------------

func TestRefresh_SingleInstance(t *testing.T) {
	inst := makeInstance()
	vnic := makeVnic("ocid1.vnic.oc1..v001", "10.0.0.5", "203.0.113.10")
	d := singleInstanceDiscovery(inst, vnic)

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs, 1)
	require.Equal(t, "OCI_"+testCompartment, tgs[0].Source)
	require.Len(t, tgs[0].Targets, 1)

	labels := tgs[0].Targets[0]
	require.Equal(t, model.LabelValue("10.0.0.5:9100"), labels[model.AddressLabel])
	require.Equal(t, model.LabelValue("ocid1.instance.oc1.iad.aaa001"), labels[ociLabelInstanceID])
	require.Equal(t, model.LabelValue("10.0.0.5"), labels[ociLabelPrivateIP])
	require.Equal(t, model.LabelValue("203.0.113.10"), labels[ociLabelPublicIP])
	require.Equal(t, model.LabelValue(testTenancy), labels[ociLabelTenancyID])
}

func TestRefresh_MultipleCompartments(t *testing.T) {
	comp1 := "ocid1.compartment.oc1..c001"
	comp2 := "ocid1.compartment.oc1..c002"
	comp3 := "ocid1.compartment.oc1..c003"

	inst1 := makeInstance(func(i *core.Instance) {
		i.Id = strPtr("ocid1.instance.oc1..i001")
		i.CompartmentId = strPtr(comp1)
	})
	inst2 := makeInstance(func(i *core.Instance) {
		i.Id = strPtr("ocid1.instance.oc1..i002")
		i.CompartmentId = strPtr(comp2)
	})
	vnic := makeVnic("ocid1.vnic.oc1..v001", "10.0.0.1", "")

	d := baseDiscovery()
	d.listCompartments = func(_ context.Context) ([]string, error) {
		return []string{comp1, comp2, comp3}, nil
	}
	d.listInstances = func(_ context.Context, compartmentID string) ([]core.Instance, error) {
		switch compartmentID {
		case comp1:
			return []core.Instance{inst1}, nil
		case comp2:
			return []core.Instance{inst2}, nil
		default:
			return nil, nil // comp3 is empty
		}
	}
	d.listVnicAttachments = func(_ context.Context, _, instanceID string) ([]core.VnicAttachment, error) {
		return []core.VnicAttachment{makeAttachment(instanceID, stringVal(vnic.Id))}, nil
	}
	d.getVnic = func(_ context.Context, _ string) (*core.Vnic, error) {
		return vnic, nil
	}

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	// One group per compartment, including the empty one.
	require.Len(t, tgs, 3)
	require.Equal(t, "OCI_"+comp1, tgs[0].Source)
	require.Len(t, tgs[0].Targets, 1)
	require.Equal(t, "OCI_"+comp2, tgs[1].Source)
	require.Len(t, tgs[1].Targets, 1)
	require.Equal(t, "OCI_"+comp3, tgs[2].Source)
	require.Empty(t, tgs[2].Targets)
}

func TestRefresh_CompartmentListError_Propagates(t *testing.T) {
	sentinel := errors.New("identity API unavailable")
	d := baseDiscovery()
	d.listCompartments = func(_ context.Context) ([]string, error) {
		return nil, sentinel
	}

	_, err := d.refresh(context.Background())
	require.ErrorIs(t, err, sentinel)
}

func TestRefresh_ListInstancesError_EmptyGroupReturned(t *testing.T) {
	// listInstances error → compartment is logged and an empty group is
	// returned so the discovery manager prunes any stale targets it still
	// has for this source.
	sentinel := errors.New("OCI API unavailable")
	d := baseDiscovery()
	d.listInstances = func(_ context.Context, _ string) ([]core.Instance, error) {
		return nil, sentinel
	}

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs, 1, "failed compartment must still return an (empty) group")
	require.Equal(t, "OCI_"+testCompartment, tgs[0].Source)
	require.Empty(t, tgs[0].Targets)
}

func TestRefresh_EmptyCompartment(t *testing.T) {
	d := baseDiscovery()

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs, 1)
	require.Empty(t, tgs[0].Targets)
}

func TestRefresh_VnicAttachmentError_InstanceSkipped(t *testing.T) {
	inst := makeInstance()
	sentinel := errors.New("VNIC list failed")

	d := baseDiscovery()
	d.listInstances = func(_ context.Context, _ string) ([]core.Instance, error) {
		return []core.Instance{inst}, nil
	}
	d.listVnicAttachments = func(_ context.Context, _, _ string) ([]core.VnicAttachment, error) {
		return nil, sentinel
	}

	// Instance is skipped, compartment group is still returned (empty targets).
	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs, 1)
	require.Empty(t, tgs[0].Targets)
}

func TestRefresh_GetVnicError_InstanceSkipped(t *testing.T) {
	inst := makeInstance()
	att := makeAttachment(stringVal(inst.Id), "ocid1.vnic.oc1..v001")
	sentinel := errors.New("GetVnic failed")

	d := baseDiscovery()
	d.listInstances = func(_ context.Context, _ string) ([]core.Instance, error) {
		return []core.Instance{inst}, nil
	}
	d.listVnicAttachments = func(_ context.Context, _, _ string) ([]core.VnicAttachment, error) {
		return []core.VnicAttachment{att}, nil
	}
	d.getVnic = func(_ context.Context, _ string) (*core.Vnic, error) {
		return nil, sentinel
	}

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs, 1)
	require.Empty(t, tgs[0].Targets)
}

func TestRefresh_TagFilterInclude_OnlyTaggedInstanceReturned(t *testing.T) {
	tagged := makeInstance(func(i *core.Instance) {
		i.Id = strPtr("ocid1.instance.oc1..tagged")
		i.FreeformTags = map[string]string{"monitoring": "enabled"}
	})
	untagged := makeInstance(func(i *core.Instance) {
		i.Id = strPtr("ocid1.instance.oc1..untagged")
	})

	vnic := makeVnic("ocid1.vnic.oc1..v001", "10.0.0.1", "")
	att := makeAttachment("ocid1.instance.oc1..tagged", stringVal(vnic.Id))

	d := baseDiscovery()
	d.tagFilterKey = "monitoring"
	d.tagFilterValue = "enabled"
	d.tagFilterAction = tagFilterActionInclude
	d.listInstances = func(_ context.Context, _ string) ([]core.Instance, error) {
		return []core.Instance{tagged, untagged}, nil
	}
	d.listVnicAttachments = func(_ context.Context, _, _ string) ([]core.VnicAttachment, error) {
		return []core.VnicAttachment{att}, nil
	}
	d.getVnic = func(_ context.Context, _ string) (*core.Vnic, error) { return vnic, nil }

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs[0].Targets, 1)
	require.Equal(t, model.LabelValue("ocid1.instance.oc1..tagged"), tgs[0].Targets[0][ociLabelInstanceID])
}

func TestRefresh_TagFilterExclude_TaggedInstanceDropped(t *testing.T) {
	disabled := makeInstance(func(i *core.Instance) {
		i.Id = strPtr("ocid1.instance.oc1..disabled")
		i.FreeformTags = map[string]string{"monitoring": "disabled"}
	})
	active := makeInstance(func(i *core.Instance) {
		i.Id = strPtr("ocid1.instance.oc1..active")
	})

	vnic := makeVnic("ocid1.vnic.oc1..v001", "10.0.0.2", "")
	att := makeAttachment("ocid1.instance.oc1..active", stringVal(vnic.Id))

	d := baseDiscovery()
	d.tagFilterKey = "monitoring"
	d.tagFilterValue = "disabled"
	d.tagFilterAction = tagFilterActionExclude
	d.listInstances = func(_ context.Context, _ string) ([]core.Instance, error) {
		return []core.Instance{disabled, active}, nil
	}
	d.listVnicAttachments = func(_ context.Context, _, _ string) ([]core.VnicAttachment, error) {
		return []core.VnicAttachment{att}, nil
	}
	d.getVnic = func(_ context.Context, _ string) (*core.Vnic, error) { return vnic, nil }

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs[0].Targets, 1)
	require.Equal(t, model.LabelValue("ocid1.instance.oc1..active"), tgs[0].Targets[0][ociLabelInstanceID])
}

func TestRefresh_TagFilter_VnicNotCalledForFilteredInstances(t *testing.T) {
	inst1 := makeInstance(func(i *core.Instance) {
		i.Id = strPtr("ocid1.instance.oc1..inst1")
		i.FreeformTags = map[string]string{"monitoring": "disabled"}
	})
	inst2 := makeInstance(func(i *core.Instance) {
		i.Id = strPtr("ocid1.instance.oc1..inst2")
		i.FreeformTags = map[string]string{"monitoring": "disabled"}
	})

	vnicCalls := 0
	d := baseDiscovery()
	d.tagFilterKey = "monitoring"
	d.tagFilterValue = "disabled"
	d.tagFilterAction = tagFilterActionExclude
	d.listInstances = func(_ context.Context, _ string) ([]core.Instance, error) {
		return []core.Instance{inst1, inst2}, nil
	}
	d.listVnicAttachments = func(_ context.Context, _, _ string) ([]core.VnicAttachment, error) {
		vnicCalls++
		return nil, nil
	}
	d.getVnic = func(_ context.Context, _ string) (*core.Vnic, error) {
		vnicCalls++
		return nil, nil
	}

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Empty(t, tgs[0].Targets)
	require.Zero(t, vnicCalls, "VNIC API must not be called for filtered-out instances")
}

func TestRefresh_NoTagFilter_AllInstancesReturned(t *testing.T) {
	instances := []core.Instance{
		makeInstance(func(i *core.Instance) { i.Id = strPtr("ocid1.instance.oc1..i1") }),
		makeInstance(func(i *core.Instance) { i.Id = strPtr("ocid1.instance.oc1..i2") }),
		makeInstance(func(i *core.Instance) { i.Id = strPtr("ocid1.instance.oc1..i3") }),
	}
	vnic := makeVnic("ocid1.vnic.oc1..v001", "10.0.0.1", "")
	att := makeAttachment("", stringVal(vnic.Id))

	d := baseDiscovery()
	d.listInstances = func(_ context.Context, _ string) ([]core.Instance, error) {
		return instances, nil
	}
	d.listVnicAttachments = func(_ context.Context, _, _ string) ([]core.VnicAttachment, error) {
		return []core.VnicAttachment{att}, nil
	}
	d.getVnic = func(_ context.Context, _ string) (*core.Vnic, error) { return vnic, nil }

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs[0].Targets, 3)
}

func TestRefresh_DefinedTagFilterInclude(t *testing.T) {
	tagged := makeInstance(func(i *core.Instance) {
		i.Id = strPtr("ocid1.instance.oc1..tagged")
		i.DefinedTags = map[string]map[string]any{
			"ops-ns": {"monitoring": "enabled"},
		}
	})
	untagged := makeInstance(func(i *core.Instance) {
		i.Id = strPtr("ocid1.instance.oc1..untagged")
	})

	vnic := makeVnic("ocid1.vnic.oc1..v001", "10.0.0.5", "")
	att := makeAttachment("ocid1.instance.oc1..tagged", stringVal(vnic.Id))

	d := baseDiscovery()
	d.tagFilterKey = "monitoring"
	d.tagFilterValue = "enabled"
	d.tagFilterAction = tagFilterActionInclude
	d.listInstances = func(_ context.Context, _ string) ([]core.Instance, error) {
		return []core.Instance{tagged, untagged}, nil
	}
	d.listVnicAttachments = func(_ context.Context, _, _ string) ([]core.VnicAttachment, error) {
		return []core.VnicAttachment{att}, nil
	}
	d.getVnic = func(_ context.Context, _ string) (*core.Vnic, error) { return vnic, nil }

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs[0].Targets, 1)
	require.Equal(t, model.LabelValue("ocid1.instance.oc1..tagged"), tgs[0].Targets[0][ociLabelInstanceID])
}

func TestRefresh_NoPrimaryVnic_InstanceDropped(t *testing.T) {
	// An instance with no resolvable primary VNIC has no usable IP, so the
	// SD drops it rather than emit an unscrapeable :port target.
	inst := makeInstance()
	nonPrimary := &core.Vnic{
		Id:        strPtr("ocid1.vnic.oc1..v001"),
		IsPrimary: boolPtr(false),
		PrivateIp: strPtr("10.0.0.5"),
	}
	att := makeAttachment(stringVal(inst.Id), stringVal(nonPrimary.Id))

	d := baseDiscovery()
	d.listInstances = func(_ context.Context, _ string) ([]core.Instance, error) {
		return []core.Instance{inst}, nil
	}
	d.listVnicAttachments = func(_ context.Context, _, _ string) ([]core.VnicAttachment, error) {
		return []core.VnicAttachment{att}, nil
	}
	d.getVnic = func(_ context.Context, _ string) (*core.Vnic, error) { return nonPrimary, nil }

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs, 1)
	require.Empty(t, tgs[0].Targets, "instance with no usable IP must be dropped")
}

func TestRefresh_InstanceWithNilIDDropped(t *testing.T) {
	// Defensive: an instance with a nil OCID would panic on dereference;
	// it must be skipped with a warning.
	inst := makeInstance(func(i *core.Instance) {
		i.Id = nil
	})

	vnicCalls := 0
	d := baseDiscovery()
	d.listInstances = func(_ context.Context, _ string) ([]core.Instance, error) {
		return []core.Instance{inst}, nil
	}
	d.listVnicAttachments = func(_ context.Context, _, _ string) ([]core.VnicAttachment, error) {
		vnicCalls++
		return nil, nil
	}

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs, 1)
	require.Empty(t, tgs[0].Targets)
	require.Zero(t, vnicCalls, "instance with nil Id must not reach VNIC lookup")
}

func TestRefresh_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	d := baseDiscovery()
	d.listCompartments = func(ctx context.Context) ([]string, error) {
		return nil, ctx.Err()
	}

	_, err := d.refresh(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

// ---- SDConfig validation ----------------------------------------------------

func TestSDConfig_Validation(t *testing.T) {
	validAPIKey := SDConfig{
		Auth:             authAPIKey,
		Region:           "us-ashburn-1",
		Tenancy:          "ocid1.tenancy.oc1..t001",
		User:             "ocid1.user.oc1..u001",
		Fingerprint:      "aa:bb:cc",
		KeyFile:          "/etc/oci/key.pem",
		TagFilterAction:  tagFilterActionInclude,
		RefreshInterval:  model.Duration(60 * time.Second),
		HTTPClientConfig: config.DefaultHTTPClientConfig,
	}

	cases := []struct {
		name    string
		mutate  func(*SDConfig)
		wantErr string
	}{
		{
			name:    "missing region",
			mutate:  func(c *SDConfig) { c.Region = "" },
			wantErr: "requires a region",
		},
		{
			name:    "api_key missing tenancy",
			mutate:  func(c *SDConfig) { c.Tenancy = "" },
			wantErr: "requires tenancy",
		},
		{
			name:    "api_key missing user",
			mutate:  func(c *SDConfig) { c.User = "" },
			wantErr: "requires user",
		},
		{
			name:    "api_key missing fingerprint",
			mutate:  func(c *SDConfig) { c.Fingerprint = "" },
			wantErr: "requires fingerprint",
		},
		{
			name:    "api_key missing key_file",
			mutate:  func(c *SDConfig) { c.KeyFile = "" },
			wantErr: "requires key_file",
		},
		{
			name: "instance_principal with api_key fields",
			mutate: func(c *SDConfig) {
				c.Auth = authInstancePrincipal
				// Tenancy still set from validAPIKey.
			},
			wantErr: "must not have tenancy",
		},
		{
			name:    "unknown auth method",
			mutate:  func(c *SDConfig) { c.Auth = "magic"; c.Tenancy = "" },
			wantErr: "unknown auth method",
		},
		{
			name: "empty string in compartments list",
			mutate: func(c *SDConfig) {
				c.Compartments = []string{"ocid1.compartment.oc1..good", ""}
			},
			wantErr: "compartments[1] must not be empty",
		},
		{
			name: "tag_filter_key without tag_filter_value",
			mutate: func(c *SDConfig) {
				c.TagFilterKey = "monitoring"
				c.TagFilterValue = ""
			},
			wantErr: "requires tag_filter_value",
		},
		{
			name: "tag_filter_action invalid",
			mutate: func(c *SDConfig) {
				c.TagFilterKey = "k"
				c.TagFilterValue = "v"
				c.TagFilterAction = "allowlist"
			},
			wantErr: "tag_filter_action must be",
		},
		{
			name:    "refresh_interval zero",
			mutate:  func(c *SDConfig) { c.RefreshInterval = 0 },
			wantErr: "refresh_interval must be positive",
		},
		{
			name:    "refresh_interval negative",
			mutate:  func(c *SDConfig) { c.RefreshInterval = model.Duration(-1 * time.Second) },
			wantErr: "refresh_interval must be positive",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validAPIKey
			tc.mutate(&cfg)
			err := cfg.validate()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestSDConfig_ValidInstancePrincipal(t *testing.T) {
	cfg := SDConfig{
		Auth:             authInstancePrincipal,
		Region:           "us-ashburn-1",
		TagFilterAction:  tagFilterActionInclude,
		RefreshInterval:  model.Duration(60 * time.Second),
		HTTPClientConfig: config.DefaultHTTPClientConfig,
	}
	require.NoError(t, cfg.validate())
}

func TestSDConfig_ValidWithExplicitCompartments(t *testing.T) {
	cfg := SDConfig{
		Auth:             authAPIKey,
		Region:           "us-ashburn-1",
		Tenancy:          "ocid1.tenancy.oc1..t001",
		User:             "ocid1.user.oc1..u001",
		Fingerprint:      "aa:bb:cc",
		KeyFile:          "/etc/oci/key.pem",
		Compartments:     []string{"ocid1.compartment.oc1..c001", "ocid1.compartment.oc1..c002"},
		TagFilterAction:  tagFilterActionInclude,
		RefreshInterval:  model.Duration(60 * time.Second),
		HTTPClientConfig: config.DefaultHTTPClientConfig,
	}
	require.NoError(t, cfg.validate())
}

func TestSDConfig_ValidAutoDiscovery(t *testing.T) {
	// Compartments empty → auto-discover. Should be valid.
	cfg := SDConfig{
		Auth:             authAPIKey,
		Region:           "us-ashburn-1",
		Tenancy:          "ocid1.tenancy.oc1..t001",
		User:             "ocid1.user.oc1..u001",
		Fingerprint:      "aa:bb:cc",
		KeyFile:          "/etc/oci/key.pem",
		TagFilterAction:  tagFilterActionInclude,
		RefreshInterval:  model.Duration(60 * time.Second),
		HTTPClientConfig: config.DefaultHTTPClientConfig,
	}
	require.NoError(t, cfg.validate())
}

func TestSDConfig_ValidExcludeFilter(t *testing.T) {
	cfg := SDConfig{
		Auth:             authAPIKey,
		Region:           "us-ashburn-1",
		Tenancy:          "ocid1.tenancy.oc1..t001",
		User:             "ocid1.user.oc1..u001",
		Fingerprint:      "aa:bb:cc",
		KeyFile:          "/etc/oci/key.pem",
		TagFilterKey:     "monitoring",
		TagFilterValue:   "disabled",
		TagFilterAction:  tagFilterActionExclude,
		RefreshInterval:  model.Duration(60 * time.Second),
		HTTPClientConfig: config.DefaultHTTPClientConfig,
	}
	require.NoError(t, cfg.validate())
}

// ---- SetDirectory -----------------------------------------------------------

func TestSDConfig_SetDirectory_RelativePath(t *testing.T) {
	dir := filepath.FromSlash("/etc/prometheus")
	cfg := SDConfig{KeyFile: filepath.FromSlash("keys/oci.pem")}
	cfg.SetDirectory(dir)
	require.Equal(t, filepath.Join(dir, "keys", "oci.pem"), cfg.KeyFile)
}

func TestSDConfig_SetDirectory_AbsolutePathUnchanged(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "oci.pem")
	cfg := SDConfig{KeyFile: abs}
	cfg.SetDirectory(filepath.FromSlash("/etc/prometheus"))
	require.Equal(t, abs, cfg.KeyFile)
}

func TestSDConfig_SetDirectory_EmptyKeyFile(t *testing.T) {
	cfg := SDConfig{}
	cfg.SetDirectory("/etc/prometheus")
	require.Empty(t, cfg.KeyFile)
}
