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
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
)

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

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

// primaryOnly is a convenience for tests that only care about the primary VNIC.
func primaryOnly(id, privateIP, publicIP string) instanceVnics {
	return instanceVnics{
		primary:    vnicInfo{id: id, privateIP: privateIP, publicIP: publicIP},
		hasPrimary: true,
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
		tenancy: testTenancy,
		port:    9100,
		logger:  slog.Default(),
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

// ---- instanceLabels ---------------------------------------------------------

func TestInstanceLabels_CoreLabels(t *testing.T) {
	inst := makeInstance()
	labels := instanceLabels(&inst, primaryOnly("ocid1.vnic.oc1..v001", "10.0.0.5", "203.0.113.1"), testTenancy, 9100)

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
		ociLabelVnicID:             "ocid1.vnic.oc1..v001",
		ociLabelPrivateIP:          "10.0.0.5",
		ociLabelPublicIP:           "203.0.113.1",
	}
	for k, want := range expected {
		require.Equal(t, want, labels[k], "label %s", k)
	}
}

func TestInstanceLabels_AddressIsPrivateIPWhenBothPresent(t *testing.T) {
	inst := makeInstance()
	labels := instanceLabels(&inst, primaryOnly("v", "10.0.0.5", "203.0.113.1"), "tenancy", 80)
	require.Equal(t, model.LabelValue("10.0.0.5:80"), labels[model.AddressLabel])
}

func TestInstanceLabels_AddressFallsBackToPublicIP(t *testing.T) {
	inst := makeInstance()
	labels := instanceLabels(&inst, primaryOnly("v", "", "203.0.113.1"), "tenancy", 80)
	require.Equal(t, model.LabelValue("203.0.113.1:80"), labels[model.AddressLabel])
}

func TestInstanceLabels_SecondaryVnicIPsExposed(t *testing.T) {
	inst := makeInstance()
	vnics := instanceVnics{
		primary:    vnicInfo{id: "ocid1.vnic.oc1..p", privateIP: "10.0.0.1", publicIP: ""},
		hasPrimary: true,
		secondary: []vnicInfo{
			{id: "ocid1.vnic.oc1..s1", privateIP: "10.0.0.2", publicIP: "203.0.113.2"},
			{id: "ocid1.vnic.oc1..s2", privateIP: "10.0.0.3"},
		},
	}
	labels := instanceLabels(&inst, vnics, "tenancy", 80)

	require.Equal(t, model.LabelValue(",10.0.0.2,10.0.0.3,"), labels[ociLabelSecondaryPrivateIPs])
	require.Equal(t, model.LabelValue(",203.0.113.2,"), labels[ociLabelSecondaryPublicIPs])
}

func TestInstanceLabels_NoSecondaryVnicLabelsWhenAbsent(t *testing.T) {
	inst := makeInstance()
	labels := instanceLabels(&inst, primaryOnly("v", "10.0.0.1", ""), "tenancy", 80)

	_, hasPrivates := labels[ociLabelSecondaryPrivateIPs]
	require.False(t, hasPrivates, "secondary private IPs label must not be set when there are no secondaries")
	_, hasPublics := labels[ociLabelSecondaryPublicIPs]
	require.False(t, hasPublics, "secondary public IPs label must not be set when there are no secondaries")
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
	labels := instanceLabels(&inst, primaryOnly("v", "10.0.0.1", ""), "tenancy", 80)

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
				"IntValue":   100,  // Must be skipped.
				"BoolValue":  true, // Must be skipped.
			},
		}
	})
	labels := instanceLabels(&inst, primaryOnly("v", "10.0.0.1", ""), "tenancy", 80)

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
	labels := instanceLabels(&inst, primaryOnly("", "10.0.0.1", ""), "tenancy", 80)

	require.Equal(t, model.LabelValue("10.0.0.1:80"), labels[model.AddressLabel])
	require.Equal(t, model.LabelValue(""), labels[ociLabelInstanceID])
	require.Equal(t, model.LabelValue(""), labels[ociLabelImageID])
}

func TestInstanceLabels_PortInAddress(t *testing.T) {
	inst := makeInstance()
	for _, port := range []int{80, 9100, 9182, 443} {
		labels := instanceLabels(&inst, primaryOnly("v", "10.0.0.1", ""), "tenancy", port)
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
	require.Equal(t, model.LabelValue("ocid1.vnic.oc1..v001"), labels[ociLabelVnicID])
	require.Equal(t, model.LabelValue("10.0.0.5"), labels[ociLabelPrivateIP])
	require.Equal(t, model.LabelValue("203.0.113.10"), labels[ociLabelPublicIP])
	require.Equal(t, model.LabelValue(testTenancy), labels[ociLabelTenancyID])
}

func TestRefresh_SecondaryVnicsExposedAsListLabels(t *testing.T) {
	inst := makeInstance()
	primary := &core.Vnic{
		Id:        strPtr("ocid1.vnic.oc1..p"),
		IsPrimary: boolPtr(true),
		PrivateIp: strPtr("10.0.0.1"),
	}
	secondary := &core.Vnic{
		Id:        strPtr("ocid1.vnic.oc1..s"),
		IsPrimary: boolPtr(false),
		PrivateIp: strPtr("10.0.0.2"),
		PublicIp:  strPtr("203.0.113.2"),
	}

	d := baseDiscovery()
	d.listInstances = func(_ context.Context, _ string) ([]core.Instance, error) {
		return []core.Instance{inst}, nil
	}
	d.listVnicAttachments = func(_ context.Context, _, _ string) ([]core.VnicAttachment, error) {
		return []core.VnicAttachment{
			makeAttachment(stringVal(inst.Id), stringVal(primary.Id)),
			makeAttachment(stringVal(inst.Id), stringVal(secondary.Id)),
		}, nil
	}
	d.getVnic = func(_ context.Context, vnicID string) (*core.Vnic, error) {
		switch vnicID {
		case stringVal(primary.Id):
			return primary, nil
		case stringVal(secondary.Id):
			return secondary, nil
		}
		return nil, errors.New("unexpected VNIC ID")
	}

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs, 1)
	require.Len(t, tgs[0].Targets, 1)

	labels := tgs[0].Targets[0]
	require.Equal(t, model.LabelValue("ocid1.vnic.oc1..p"), labels[ociLabelVnicID])
	require.Equal(t, model.LabelValue(",10.0.0.2,"), labels[ociLabelSecondaryPrivateIPs])
	require.Equal(t, model.LabelValue(",203.0.113.2,"), labels[ociLabelSecondaryPublicIPs])
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
	// listInstances error -> compartment is logged and an empty group is
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

// ---- listAllCompartments ----------------------------------------------------

// fakeCompartmentTree drives listAllCompartments via a compartmentPaginator
// stub. Each node has a list of pages, each page a list of (id, active,
// hasNext) tuples. Setting failChildrenOf to a node ID makes paginate return
// an error when that node is queried, simulating a permission gap.
type fakeCompartmentTree struct {
	// children maps parent compartment ID -> ordered direct children.
	children map[string][]identity.Compartment
	// pageSize bins children into pages of this size to exercise pagination.
	pageSize int
	// permissionDenied is the set of parent IDs whose list call should fail.
	permissionDenied map[string]bool
	// errOnce records first call so cancelled-context tests can observe it.
	calls map[string]int
}

func (f *fakeCompartmentTree) paginate(ctx context.Context, parentID string, page *string) ([]identity.Compartment, *string, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[parentID]++

	if f.permissionDenied[parentID] {
		return nil, nil, errors.New("permission denied")
	}

	all := f.children[parentID]
	if f.pageSize <= 0 || f.pageSize >= len(all) {
		return all, nil, nil
	}

	start := 0
	if page != nil {
		// Page tokens are 1-based indices into the slice.
		idx, err := strconv.Atoi(*page)
		if err != nil {
			return nil, nil, err
		}
		start = idx
	}
	end := start + f.pageSize
	if end >= len(all) {
		return all[start:], nil, nil
	}
	next := strconv.Itoa(end)
	return all[start:end], &next, nil
}

func activeComp(id string) identity.Compartment {
	return identity.Compartment{Id: strPtr(id), LifecycleState: identity.CompartmentLifecycleStateActive}
}

func inactiveComp(id string) identity.Compartment {
	return identity.Compartment{Id: strPtr(id), LifecycleState: identity.CompartmentLifecycleStateInactive}
}

func TestListAllCompartments_BFSIncludesRoot(t *testing.T) {
	tree := &fakeCompartmentTree{
		children: map[string][]identity.Compartment{
			"root": {activeComp("c1"), activeComp("c2")},
			"c1":   {activeComp("c1a")},
		},
	}

	got, err := listAllCompartments(context.Background(), tree.paginate, "root", promslog.NewNopLogger())
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"root", "c1", "c2", "c1a"}, got)
}

func TestListAllCompartments_PagedResults(t *testing.T) {
	tree := &fakeCompartmentTree{
		children: map[string][]identity.Compartment{
			"root": {activeComp("c1"), activeComp("c2"), activeComp("c3"), activeComp("c4"), activeComp("c5")},
		},
		pageSize: 2,
	}

	got, err := listAllCompartments(context.Background(), tree.paginate, "root", promslog.NewNopLogger())
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"root", "c1", "c2", "c3", "c4", "c5"}, got)
	require.Equal(t, 3, tree.calls["root"], "5 children with page size 2 must take 3 calls")
}

func TestListAllCompartments_InactiveCompartmentSkipped(t *testing.T) {
	tree := &fakeCompartmentTree{
		children: map[string][]identity.Compartment{
			"root": {activeComp("c1"), inactiveComp("c2-dead"), activeComp("c3")},
		},
	}

	got, err := listAllCompartments(context.Background(), tree.paginate, "root", promslog.NewNopLogger())
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"root", "c1", "c3"}, got)
}

func TestListAllCompartments_CycleVisitedOnce(t *testing.T) {
	// A child that references back to root must not cause an infinite loop.
	tree := &fakeCompartmentTree{
		children: map[string][]identity.Compartment{
			"root": {activeComp("c1")},
			"c1":   {activeComp("root")}, // Cycle.
		},
	}

	got, err := listAllCompartments(context.Background(), tree.paginate, "root", promslog.NewNopLogger())
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"root", "c1"}, got)
	require.Equal(t, 1, tree.calls["root"], "root must be listed only once even when referenced as a child")
}

func TestListAllCompartments_PermissionErrorSkipsSubtree(t *testing.T) {
	tree := &fakeCompartmentTree{
		children: map[string][]identity.Compartment{
			"root": {activeComp("c1"), activeComp("c2")},
			"c1":   {activeComp("c1a")},
			// c2 denied; its subtree must not abort the walk.
		},
		permissionDenied: map[string]bool{"c2": true},
	}

	got, err := listAllCompartments(context.Background(), tree.paginate, "root", promslog.NewNopLogger())
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"root", "c1", "c2", "c1a"}, got)
}

func TestListAllCompartments_CanceledContextAborts(t *testing.T) {
	// A cancelled context must abort the walk immediately rather than be
	// silently treated as a permission gap.
	tree := &fakeCompartmentTree{
		children: map[string][]identity.Compartment{
			"root": {activeComp("c1")},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := listAllCompartments(ctx, tree.paginate, "root", promslog.NewNopLogger())
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
			name: "api_key with both key_passphrase and key_passphrase_file",
			mutate: func(c *SDConfig) {
				c.KeyPassphrase = "secret"
				c.KeyPassphraseFile = "/etc/oci/passphrase"
			},
			wantErr: "at most one of key_passphrase and key_passphrase_file",
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
			name: "instance_principal with key_passphrase_file",
			mutate: func(c *SDConfig) {
				c.Auth = authInstancePrincipal
				c.Tenancy = ""
				c.User = ""
				c.Fingerprint = ""
				c.KeyFile = ""
				c.KeyPassphraseFile = "/etc/oci/passphrase"
			},
			wantErr: "must not have",
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
		RefreshInterval:  model.Duration(60 * time.Second),
		HTTPClientConfig: config.DefaultHTTPClientConfig,
	}
	require.NoError(t, cfg.validate())
}

func TestSDConfig_ValidAutoDiscovery(t *testing.T) {
	// Compartments empty -> auto-discover. Should be valid.
	cfg := SDConfig{
		Auth:             authAPIKey,
		Region:           "us-ashburn-1",
		Tenancy:          "ocid1.tenancy.oc1..t001",
		User:             "ocid1.user.oc1..u001",
		Fingerprint:      "aa:bb:cc",
		KeyFile:          "/etc/oci/key.pem",
		RefreshInterval:  model.Duration(60 * time.Second),
		HTTPClientConfig: config.DefaultHTTPClientConfig,
	}
	require.NoError(t, cfg.validate())
}

func TestSDConfig_ValidWithKeyPassphraseFile(t *testing.T) {
	cfg := SDConfig{
		Auth:              authAPIKey,
		Region:            "us-ashburn-1",
		Tenancy:           "ocid1.tenancy.oc1..t001",
		User:              "ocid1.user.oc1..u001",
		Fingerprint:       "aa:bb:cc",
		KeyFile:           "/etc/oci/key.pem",
		KeyPassphraseFile: "/etc/oci/passphrase",
		RefreshInterval:   model.Duration(60 * time.Second),
		HTTPClientConfig:  config.DefaultHTTPClientConfig,
	}
	require.NoError(t, cfg.validate())
}

// ---- resolveKeyPassphrase ---------------------------------------------------

func TestResolveKeyPassphrase_InlinePreferredWhenSetAlone(t *testing.T) {
	got, err := resolveKeyPassphrase(&SDConfig{KeyPassphrase: "topsecret"})
	require.NoError(t, err)
	require.Equal(t, "topsecret", got)
}

func TestResolveKeyPassphrase_FileTrimsTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pass")
	require.NoError(t, writeFile(path, "topsecret\n"))

	got, err := resolveKeyPassphrase(&SDConfig{KeyPassphraseFile: path})
	require.NoError(t, err)
	require.Equal(t, "topsecret", got)
}

func TestResolveKeyPassphrase_FileMissing(t *testing.T) {
	_, err := resolveKeyPassphrase(&SDConfig{KeyPassphraseFile: filepath.Join(t.TempDir(), "nope")})
	require.Error(t, err)
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

func TestSDConfig_SetDirectory_KeyPassphraseFileJoined(t *testing.T) {
	dir := filepath.FromSlash("/etc/prometheus")
	cfg := SDConfig{KeyPassphraseFile: filepath.FromSlash("keys/oci.pass")}
	cfg.SetDirectory(dir)
	require.Equal(t, filepath.Join(dir, "keys", "oci.pass"), cfg.KeyPassphraseFile)
}
