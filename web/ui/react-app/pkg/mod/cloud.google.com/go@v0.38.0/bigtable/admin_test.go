// Copyright 2015 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigtable

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/golang/protobuf/proto"
	"google.golang.org/api/iterator"
	btapb "google.golang.org/genproto/googleapis/bigtable/admin/v2"
)

func TestAdminIntegration(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, _ := context.WithTimeout(context.Background(), timeout)

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	if iAdminClient != nil {
		defer iAdminClient.Close()

		iInfo, err := iAdminClient.InstanceInfo(ctx, adminClient.instance)
		if err != nil {
			t.Errorf("InstanceInfo: %v", err)
		}
		if iInfo.Name != adminClient.instance {
			t.Errorf("InstanceInfo returned name %#v, want %#v", iInfo.Name, adminClient.instance)
		}
	}

	list := func() []string {
		tbls, err := adminClient.Tables(ctx)
		if err != nil {
			t.Fatalf("Fetching list of tables: %v", err)
		}
		sort.Strings(tbls)
		return tbls
	}
	containsAll := func(got, want []string) bool {
		gotSet := make(map[string]bool)

		for _, s := range got {
			gotSet[s] = true
		}
		for _, s := range want {
			if !gotSet[s] {
				return false
			}
		}
		return true
	}

	defer adminClient.DeleteTable(ctx, "mytable")

	if err := adminClient.CreateTable(ctx, "mytable"); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	defer adminClient.DeleteTable(ctx, "myothertable")

	if err := adminClient.CreateTable(ctx, "myothertable"); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	if got, want := list(), []string{"myothertable", "mytable"}; !containsAll(got, want) {
		t.Errorf("adminClient.Tables returned %#v, want %#v", got, want)
	}

	must(adminClient.WaitForReplication(ctx, "mytable"))

	if err := adminClient.DeleteTable(ctx, "myothertable"); err != nil {
		t.Fatalf("Deleting table: %v", err)
	}
	tables := list()
	if got, want := tables, []string{"mytable"}; !containsAll(got, want) {
		t.Errorf("adminClient.Tables returned %#v, want %#v", got, want)
	}
	if got, unwanted := tables, []string{"myothertable"}; containsAll(got, unwanted) {
		t.Errorf("adminClient.Tables return %#v. unwanted %#v", got, unwanted)
	}

	tblConf := TableConf{
		TableID: "conftable",
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
	}
	if err := adminClient.CreateTableFromConf(ctx, &tblConf); err != nil {
		t.Fatalf("Creating table from TableConf: %v", err)
	}
	defer adminClient.DeleteTable(ctx, tblConf.TableID)

	tblInfo, err := adminClient.TableInfo(ctx, tblConf.TableID)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}
	sort.Strings(tblInfo.Families)
	wantFams := []string{"fam1", "fam2"}
	if !testutil.Equal(tblInfo.Families, wantFams) {
		t.Errorf("Column family mismatch, got %v, want %v", tblInfo.Families, wantFams)
	}

	// Populate mytable and drop row ranges
	if err = adminClient.CreateColumnFamily(ctx, "mytable", "cf"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}

	client, err := testEnv.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	tbl := client.Open("mytable")

	prefixes := []string{"a", "b", "c"}
	for _, prefix := range prefixes {
		for i := 0; i < 5; i++ {
			mut := NewMutation()
			mut.Set("cf", "col", 1000, []byte("1"))
			if err := tbl.Apply(ctx, fmt.Sprintf("%v-%v", prefix, i), mut); err != nil {
				t.Fatalf("Mutating row: %v", err)
			}
		}
	}

	if err = adminClient.DropRowRange(ctx, "mytable", "a"); err != nil {
		t.Errorf("DropRowRange a: %v", err)
	}
	if err = adminClient.DropRowRange(ctx, "mytable", "c"); err != nil {
		t.Errorf("DropRowRange c: %v", err)
	}
	if err = adminClient.DropRowRange(ctx, "mytable", "x"); err != nil {
		t.Errorf("DropRowRange x: %v", err)
	}

	var gotRowCount int
	must(tbl.ReadRows(ctx, RowRange{}, func(row Row) bool {
		gotRowCount++
		if !strings.HasPrefix(row.Key(), "b") {
			t.Errorf("Invalid row after dropping range: %v", row)
		}
		return true
	}))
	if gotRowCount != 5 {
		t.Errorf("Invalid row count after dropping range: got %v, want %v", gotRowCount, 5)
	}
}

func TestInstanceUpdate(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}

	defer adminClient.Close()

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}

	if iAdminClient == nil {
		return
	}

	defer iAdminClient.Close()

	iInfo, err := iAdminClient.InstanceInfo(ctx, adminClient.instance)
	if err != nil {
		t.Errorf("InstanceInfo: %v", err)
	}

	if iInfo.Name != adminClient.instance {
		t.Errorf("InstanceInfo returned name %#v, want %#v", iInfo.Name, adminClient.instance)
	}

	if iInfo.DisplayName != adminClient.instance {
		t.Errorf("InstanceInfo returned name %#v, want %#v", iInfo.Name, adminClient.instance)
	}

	const numNodes = 4
	// update cluster nodes
	if err := iAdminClient.UpdateCluster(ctx, adminClient.instance, testEnv.Config().Cluster, int32(numNodes)); err != nil {
		t.Errorf("UpdateCluster: %v", err)
	}

	// get cluster after updating
	cis, err := iAdminClient.GetCluster(ctx, adminClient.instance, testEnv.Config().Cluster)
	if err != nil {
		t.Errorf("GetCluster %v", err)
	}
	if cis.ServeNodes != int(numNodes) {
		t.Errorf("ServeNodes returned %d, want %d", cis.ServeNodes, int(numNodes))
	}
}

func TestAdminSnapshotIntegration(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support snapshots")
	}

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, _ := context.WithTimeout(context.Background(), timeout)

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	table := testEnv.Config().Table
	cluster := testEnv.Config().Cluster

	list := func(cluster string) ([]*SnapshotInfo, error) {
		infos := []*SnapshotInfo(nil)

		it := adminClient.Snapshots(ctx, cluster)
		for {
			s, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, err
			}
			infos = append(infos, s)
		}
		return infos, err
	}

	// Delete the table at the end of the test. Schedule ahead of time
	// in case the client fails
	defer adminClient.DeleteTable(ctx, table)

	if err := adminClient.CreateTable(ctx, table); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	// Precondition: no snapshots
	snapshots, err := list(cluster)
	if err != nil {
		t.Fatalf("Initial snapshot list: %v", err)
	}
	if got, want := len(snapshots), 0; got != want {
		t.Fatalf("Initial snapshot list len: %d, want: %d", got, want)
	}

	// Create snapshot
	defer adminClient.DeleteSnapshot(ctx, cluster, "mysnapshot")

	if err = adminClient.SnapshotTable(ctx, table, cluster, "mysnapshot", 5*time.Hour); err != nil {
		t.Fatalf("Creating snaphot: %v", err)
	}

	// List snapshot
	snapshots, err = list(cluster)
	if err != nil {
		t.Fatalf("Listing snapshots: %v", err)
	}
	if got, want := len(snapshots), 1; got != want {
		t.Fatalf("Listing snapshot count: %d, want: %d", got, want)
	}
	if got, want := snapshots[0].Name, "mysnapshot"; got != want {
		t.Fatalf("Snapshot name: %s, want: %s", got, want)
	}
	if got, want := snapshots[0].SourceTable, table; got != want {
		t.Fatalf("Snapshot SourceTable: %s, want: %s", got, want)
	}
	if got, want := snapshots[0].DeleteTime, snapshots[0].CreateTime.Add(5*time.Hour); math.Abs(got.Sub(want).Minutes()) > 1 {
		t.Fatalf("Snapshot DeleteTime: %s, want: %s", got, want)
	}

	// Get snapshot
	snapshot, err := adminClient.SnapshotInfo(ctx, cluster, "mysnapshot")
	if err != nil {
		t.Fatalf("SnapshotInfo: %v", snapshot)
	}
	if got, want := *snapshot, *snapshots[0]; got != want {
		t.Fatalf("SnapshotInfo: %v, want: %v", got, want)
	}

	// Restore
	restoredTable := table + "-restored"
	defer adminClient.DeleteTable(ctx, restoredTable)
	if err = adminClient.CreateTableFromSnapshot(ctx, restoredTable, cluster, "mysnapshot"); err != nil {
		t.Fatalf("CreateTableFromSnapshot: %v", err)
	}
	if _, err := adminClient.TableInfo(ctx, restoredTable); err != nil {
		t.Fatalf("Restored TableInfo: %v", err)
	}

	// Delete snapshot
	if err = adminClient.DeleteSnapshot(ctx, cluster, "mysnapshot"); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}
	snapshots, err = list(cluster)
	if err != nil {
		t.Fatalf("List after Delete: %v", err)
	}
	if got, want := len(snapshots), 0; got != want {
		t.Fatalf("List after delete len: %d, want: %d", got, want)
	}
}

func TestGranularity(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, _ := context.WithTimeout(context.Background(), timeout)

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	list := func() []string {
		tbls, err := adminClient.Tables(ctx)
		if err != nil {
			t.Fatalf("Fetching list of tables: %v", err)
		}
		sort.Strings(tbls)
		return tbls
	}
	containsAll := func(got, want []string) bool {
		gotSet := make(map[string]bool)

		for _, s := range got {
			gotSet[s] = true
		}
		for _, s := range want {
			if !gotSet[s] {
				return false
			}
		}
		return true
	}

	defer adminClient.DeleteTable(ctx, "mytable")

	if err := adminClient.CreateTable(ctx, "mytable"); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	tables := list()
	if got, want := tables, []string{"mytable"}; !containsAll(got, want) {
		t.Errorf("adminClient.Tables returned %#v, want %#v", got, want)
	}

	// calling ModifyColumnFamilies to check the granularity of table
	prefix := adminClient.instancePrefix()
	req := &btapb.ModifyColumnFamiliesRequest{
		Name: prefix + "/tables/" + "mytable",
		Modifications: []*btapb.ModifyColumnFamiliesRequest_Modification{{
			Id:  "cf",
			Mod: &btapb.ModifyColumnFamiliesRequest_Modification_Create{&btapb.ColumnFamily{}},
		}},
	}
	table, err := adminClient.tClient.ModifyColumnFamilies(ctx, req)
	if err != nil {
		t.Fatalf("Creating column family: %v", err)
	}
	if table.Granularity != btapb.Table_TimestampGranularity(btapb.Table_MILLIS) {
		t.Errorf("ModifyColumnFamilies returned granularity %#v, want %#v", table.Granularity, btapb.Table_TimestampGranularity(btapb.Table_MILLIS))
	}
}

func TestInstanceAdminClient_AppProfile(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}

	if iAdminClient == nil {
		return
	}

	defer iAdminClient.Close()
	profile := ProfileConf{
		ProfileID:     "app_profile1",
		InstanceID:    adminClient.instance,
		ClusterID:     testEnv.Config().Cluster,
		Description:   "creating new app profile 1",
		RoutingPolicy: SingleClusterRouting,
	}

	createdProfile, err := iAdminClient.CreateAppProfile(ctx, profile)
	if err != nil {
		t.Fatalf("Creating app profile: %v", err)

	}

	gotProfile, err := iAdminClient.GetAppProfile(ctx, adminClient.instance, "app_profile1")

	if err != nil {
		t.Fatalf("Get app profile: %v", err)
	}

	if !proto.Equal(createdProfile, gotProfile) {
		t.Fatalf("created profile: %s, got profile: %s", createdProfile.Name, gotProfile.Name)

	}

	list := func(instanceID string) ([]*btapb.AppProfile, error) {
		profiles := []*btapb.AppProfile(nil)

		it := iAdminClient.ListAppProfiles(ctx, instanceID)
		for {
			s, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, err
			}
			profiles = append(profiles, s)
		}
		return profiles, err
	}

	profiles, err := list(adminClient.instance)
	if err != nil {
		t.Fatalf("List app profile: %v", err)
	}

	if got, want := len(profiles), 1; got != want {
		t.Fatalf("Initial app profile list len: %d, want: %d", got, want)
	}

	for _, test := range []struct {
		desc   string
		uattrs ProfileAttrsToUpdate
		want   *btapb.AppProfile // nil means error
	}{
		{
			desc:   "empty update",
			uattrs: ProfileAttrsToUpdate{},
			want:   nil,
		},

		{
			desc:   "empty description update",
			uattrs: ProfileAttrsToUpdate{Description: ""},
			want: &btapb.AppProfile{
				Name:          gotProfile.Name,
				Description:   "",
				RoutingPolicy: gotProfile.RoutingPolicy,
				Etag:          gotProfile.Etag},
		},
		{
			desc: "routing update",
			uattrs: ProfileAttrsToUpdate{
				RoutingPolicy: SingleClusterRouting,
				ClusterID:     testEnv.Config().Cluster,
			},
			want: &btapb.AppProfile{
				Name:        gotProfile.Name,
				Description: "",
				Etag:        gotProfile.Etag,
				RoutingPolicy: &btapb.AppProfile_SingleClusterRouting_{
					SingleClusterRouting: &btapb.AppProfile_SingleClusterRouting{
						ClusterId: testEnv.Config().Cluster,
					}},
			},
		},
	} {
		err = iAdminClient.UpdateAppProfile(ctx, adminClient.instance, "app_profile1", test.uattrs)
		if err != nil {
			if test.want != nil {
				t.Errorf("%s: %v", test.desc, err)
			}
			continue
		}
		if err == nil && test.want == nil {
			t.Errorf("%s: got nil, want error", test.desc)
			continue
		}

		got, _ := iAdminClient.GetAppProfile(ctx, adminClient.instance, "app_profile1")

		if !proto.Equal(got, test.want) {
			t.Fatalf("%s : got profile : %v, want profile: %v", test.desc, gotProfile, test.want)
		}

	}

	err = iAdminClient.DeleteAppProfile(ctx, adminClient.instance, "app_profile1")
	if err != nil {
		t.Fatalf("Delete app profile: %v", err)
	}

}
