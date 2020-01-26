/*
Copyright 2015 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bigtable

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/bigtable/internal/gax"
	btopt "cloud.google.com/go/bigtable/internal/option"
	"cloud.google.com/go/iam"
	"cloud.google.com/go/internal/optional"
	"cloud.google.com/go/longrunning"
	lroauto "cloud.google.com/go/longrunning/autogen"
	"github.com/golang/protobuf/ptypes"
	durpb "github.com/golang/protobuf/ptypes/duration"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	btapb "google.golang.org/genproto/googleapis/bigtable/admin/v2"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const adminAddr = "bigtableadmin.googleapis.com:443"

// AdminClient is a client type for performing admin operations within a specific instance.
type AdminClient struct {
	conn      *grpc.ClientConn
	tClient   btapb.BigtableTableAdminClient
	lroClient *lroauto.OperationsClient

	project, instance string

	// Metadata to be sent with each request.
	md metadata.MD
}

// NewAdminClient creates a new AdminClient for a given project and instance.
func NewAdminClient(ctx context.Context, project, instance string, opts ...option.ClientOption) (*AdminClient, error) {
	o, err := btopt.DefaultClientOptions(adminAddr, AdminScope, clientUserAgent)
	if err != nil {
		return nil, err
	}
	// Need to add scopes for long running operations (for create table & snapshots)
	o = append(o, option.WithScopes(cloudresourcemanager.CloudPlatformScope))
	o = append(o, opts...)
	conn, err := gtransport.Dial(ctx, o...)
	if err != nil {
		return nil, fmt.Errorf("dialing: %v", err)
	}

	lroClient, err := lroauto.NewOperationsClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		// This error "should not happen", since we are just reusing old connection
		// and never actually need to dial.
		// If this does happen, we could leak conn. However, we cannot close conn:
		// If the user invoked the function with option.WithGRPCConn,
		// we would close a connection that's still in use.
		// TODO(pongad): investigate error conditions.
		return nil, err
	}

	return &AdminClient{
		conn:      conn,
		tClient:   btapb.NewBigtableTableAdminClient(conn),
		lroClient: lroClient,
		project:   project,
		instance:  instance,
		md:        metadata.Pairs(resourcePrefixHeader, fmt.Sprintf("projects/%s/instances/%s", project, instance)),
	}, nil
}

// Close closes the AdminClient.
func (ac *AdminClient) Close() error {
	return ac.conn.Close()
}

func (ac *AdminClient) instancePrefix() string {
	return fmt.Sprintf("projects/%s/instances/%s", ac.project, ac.instance)
}

// Tables returns a list of the tables in the instance.
func (ac *AdminClient) Tables(ctx context.Context) ([]string, error) {
	ctx = mergeOutgoingMetadata(ctx, ac.md)
	prefix := ac.instancePrefix()
	req := &btapb.ListTablesRequest{
		Parent: prefix,
	}

	var res *btapb.ListTablesResponse
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		res, err = ac.tClient.ListTables(ctx, req)
		return err
	}, retryOptions...)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(res.Tables))
	for _, tbl := range res.Tables {
		names = append(names, strings.TrimPrefix(tbl.Name, prefix+"/tables/"))
	}
	return names, nil
}

// TableConf contains all of the information necessary to create a table with column families.
type TableConf struct {
	TableID   string
	SplitKeys []string
	// Families is a map from family name to GCPolicy
	Families map[string]GCPolicy
}

// CreateTable creates a new table in the instance.
// This method may return before the table's creation is complete.
func (ac *AdminClient) CreateTable(ctx context.Context, table string) error {
	return ac.CreateTableFromConf(ctx, &TableConf{TableID: table})
}

// CreatePresplitTable creates a new table in the instance.
// The list of row keys will be used to initially split the table into multiple tablets.
// Given two split keys, "s1" and "s2", three tablets will be created,
// spanning the key ranges: [, s1), [s1, s2), [s2, ).
// This method may return before the table's creation is complete.
func (ac *AdminClient) CreatePresplitTable(ctx context.Context, table string, splitKeys []string) error {
	return ac.CreateTableFromConf(ctx, &TableConf{TableID: table, SplitKeys: splitKeys})
}

// CreateTableFromConf creates a new table in the instance from the given configuration.
func (ac *AdminClient) CreateTableFromConf(ctx context.Context, conf *TableConf) error {
	ctx = mergeOutgoingMetadata(ctx, ac.md)
	var reqSplits []*btapb.CreateTableRequest_Split
	for _, split := range conf.SplitKeys {
		reqSplits = append(reqSplits, &btapb.CreateTableRequest_Split{Key: []byte(split)})
	}
	var tbl btapb.Table
	if conf.Families != nil {
		tbl.ColumnFamilies = make(map[string]*btapb.ColumnFamily)
		for fam, policy := range conf.Families {
			tbl.ColumnFamilies[fam] = &btapb.ColumnFamily{GcRule: policy.proto()}
		}
	}
	prefix := ac.instancePrefix()
	req := &btapb.CreateTableRequest{
		Parent:        prefix,
		TableId:       conf.TableID,
		Table:         &tbl,
		InitialSplits: reqSplits,
	}
	_, err := ac.tClient.CreateTable(ctx, req)
	return err
}

// CreateColumnFamily creates a new column family in a table.
func (ac *AdminClient) CreateColumnFamily(ctx context.Context, table, family string) error {
	// TODO(dsymonds): Permit specifying gcexpr and any other family settings.
	ctx = mergeOutgoingMetadata(ctx, ac.md)
	prefix := ac.instancePrefix()
	req := &btapb.ModifyColumnFamiliesRequest{
		Name: prefix + "/tables/" + table,
		Modifications: []*btapb.ModifyColumnFamiliesRequest_Modification{{
			Id:  family,
			Mod: &btapb.ModifyColumnFamiliesRequest_Modification_Create{Create: &btapb.ColumnFamily{}},
		}},
	}
	_, err := ac.tClient.ModifyColumnFamilies(ctx, req)
	return err
}

// DeleteTable deletes a table and all of its data.
func (ac *AdminClient) DeleteTable(ctx context.Context, table string) error {
	ctx = mergeOutgoingMetadata(ctx, ac.md)
	prefix := ac.instancePrefix()
	req := &btapb.DeleteTableRequest{
		Name: prefix + "/tables/" + table,
	}
	_, err := ac.tClient.DeleteTable(ctx, req)
	return err
}

// DeleteColumnFamily deletes a column family in a table and all of its data.
func (ac *AdminClient) DeleteColumnFamily(ctx context.Context, table, family string) error {
	ctx = mergeOutgoingMetadata(ctx, ac.md)
	prefix := ac.instancePrefix()
	req := &btapb.ModifyColumnFamiliesRequest{
		Name: prefix + "/tables/" + table,
		Modifications: []*btapb.ModifyColumnFamiliesRequest_Modification{{
			Id:  family,
			Mod: &btapb.ModifyColumnFamiliesRequest_Modification_Drop{Drop: true},
		}},
	}
	_, err := ac.tClient.ModifyColumnFamilies(ctx, req)
	return err
}

// TableInfo represents information about a table.
type TableInfo struct {
	// DEPRECATED - This field is deprecated. Please use FamilyInfos instead.
	Families    []string
	FamilyInfos []FamilyInfo
}

// FamilyInfo represents information about a column family.
type FamilyInfo struct {
	Name     string
	GCPolicy string
}

// TableInfo retrieves information about a table.
func (ac *AdminClient) TableInfo(ctx context.Context, table string) (*TableInfo, error) {
	ctx = mergeOutgoingMetadata(ctx, ac.md)
	prefix := ac.instancePrefix()
	req := &btapb.GetTableRequest{
		Name: prefix + "/tables/" + table,
	}

	var res *btapb.Table

	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		res, err = ac.tClient.GetTable(ctx, req)
		return err
	}, retryOptions...)
	if err != nil {
		return nil, err
	}

	ti := &TableInfo{}
	for name, fam := range res.ColumnFamilies {
		ti.Families = append(ti.Families, name)
		ti.FamilyInfos = append(ti.FamilyInfos, FamilyInfo{Name: name, GCPolicy: GCRuleToString(fam.GcRule)})
	}
	return ti, nil
}

// SetGCPolicy specifies which cells in a column family should be garbage collected.
// GC executes opportunistically in the background; table reads may return data
// matching the GC policy.
func (ac *AdminClient) SetGCPolicy(ctx context.Context, table, family string, policy GCPolicy) error {
	ctx = mergeOutgoingMetadata(ctx, ac.md)
	prefix := ac.instancePrefix()
	req := &btapb.ModifyColumnFamiliesRequest{
		Name: prefix + "/tables/" + table,
		Modifications: []*btapb.ModifyColumnFamiliesRequest_Modification{{
			Id:  family,
			Mod: &btapb.ModifyColumnFamiliesRequest_Modification_Update{Update: &btapb.ColumnFamily{GcRule: policy.proto()}},
		}},
	}
	_, err := ac.tClient.ModifyColumnFamilies(ctx, req)
	return err
}

// DropRowRange permanently deletes a row range from the specified table.
func (ac *AdminClient) DropRowRange(ctx context.Context, table, rowKeyPrefix string) error {
	ctx = mergeOutgoingMetadata(ctx, ac.md)
	prefix := ac.instancePrefix()
	req := &btapb.DropRowRangeRequest{
		Name:   prefix + "/tables/" + table,
		Target: &btapb.DropRowRangeRequest_RowKeyPrefix{RowKeyPrefix: []byte(rowKeyPrefix)},
	}
	_, err := ac.tClient.DropRowRange(ctx, req)
	return err
}

// CreateTableFromSnapshot creates a table from snapshot.
// The table will be created in the same cluster as the snapshot.
//
// This is a private alpha release of Cloud Bigtable snapshots. This feature
// is not currently available to most Cloud Bigtable customers. This feature
// might be changed in backward-incompatible ways and is not recommended for
// production use. It is not subject to any SLA or deprecation policy.
func (ac *AdminClient) CreateTableFromSnapshot(ctx context.Context, table, cluster, snapshot string) error {
	ctx = mergeOutgoingMetadata(ctx, ac.md)
	prefix := ac.instancePrefix()
	snapshotPath := prefix + "/clusters/" + cluster + "/snapshots/" + snapshot

	req := &btapb.CreateTableFromSnapshotRequest{
		Parent:         prefix,
		TableId:        table,
		SourceSnapshot: snapshotPath,
	}
	op, err := ac.tClient.CreateTableFromSnapshot(ctx, req)
	if err != nil {
		return err
	}
	resp := btapb.Table{}
	return longrunning.InternalNewOperation(ac.lroClient, op).Wait(ctx, &resp)
}

// DefaultSnapshotDuration is the default TTL for a snapshot.
const DefaultSnapshotDuration time.Duration = 0

// SnapshotTable creates a new snapshot in the specified cluster from the
// specified source table. Setting the TTL to `DefaultSnapshotDuration` will
// use the server side default for the duration.
//
// This is a private alpha release of Cloud Bigtable snapshots. This feature
// is not currently available to most Cloud Bigtable customers. This feature
// might be changed in backward-incompatible ways and is not recommended for
// production use. It is not subject to any SLA or deprecation policy.
func (ac *AdminClient) SnapshotTable(ctx context.Context, table, cluster, snapshot string, ttl time.Duration) error {
	ctx = mergeOutgoingMetadata(ctx, ac.md)
	prefix := ac.instancePrefix()

	var ttlProto *durpb.Duration

	if ttl > 0 {
		ttlProto = ptypes.DurationProto(ttl)
	}

	req := &btapb.SnapshotTableRequest{
		Name:       prefix + "/tables/" + table,
		Cluster:    prefix + "/clusters/" + cluster,
		SnapshotId: snapshot,
		Ttl:        ttlProto,
	}

	op, err := ac.tClient.SnapshotTable(ctx, req)
	if err != nil {
		return err
	}
	resp := btapb.Snapshot{}
	return longrunning.InternalNewOperation(ac.lroClient, op).Wait(ctx, &resp)
}

// Snapshots returns a SnapshotIterator for iterating over the snapshots in a cluster.
// To list snapshots across all of the clusters in the instance specify "-" as the cluster.
//
// This is a private alpha release of Cloud Bigtable snapshots. This feature is not
// currently available to most Cloud Bigtable customers. This feature might be
// changed in backward-incompatible ways and is not recommended for production use.
// It is not subject to any SLA or deprecation policy.
func (ac *AdminClient) Snapshots(ctx context.Context, cluster string) *SnapshotIterator {
	ctx = mergeOutgoingMetadata(ctx, ac.md)
	prefix := ac.instancePrefix()
	clusterPath := prefix + "/clusters/" + cluster

	it := &SnapshotIterator{}
	req := &btapb.ListSnapshotsRequest{
		Parent: clusterPath,
	}

	fetch := func(pageSize int, pageToken string) (string, error) {
		req.PageToken = pageToken
		if pageSize > math.MaxInt32 {
			req.PageSize = math.MaxInt32
		} else {
			req.PageSize = int32(pageSize)
		}

		var resp *btapb.ListSnapshotsResponse
		err := gax.Invoke(ctx, func(ctx context.Context) error {
			var err error
			resp, err = ac.tClient.ListSnapshots(ctx, req)
			return err
		}, retryOptions...)
		if err != nil {
			return "", err
		}
		for _, s := range resp.Snapshots {
			snapshotInfo, err := newSnapshotInfo(s)
			if err != nil {
				return "", fmt.Errorf("failed to parse snapshot proto %v", err)
			}
			it.items = append(it.items, snapshotInfo)
		}
		return resp.NextPageToken, nil
	}
	bufLen := func() int { return len(it.items) }
	takeBuf := func() interface{} { b := it.items; it.items = nil; return b }

	it.pageInfo, it.nextFunc = iterator.NewPageInfo(fetch, bufLen, takeBuf)

	return it
}

func newSnapshotInfo(snapshot *btapb.Snapshot) (*SnapshotInfo, error) {
	nameParts := strings.Split(snapshot.Name, "/")
	name := nameParts[len(nameParts)-1]
	tablePathParts := strings.Split(snapshot.SourceTable.Name, "/")
	tableID := tablePathParts[len(tablePathParts)-1]

	createTime, err := ptypes.Timestamp(snapshot.CreateTime)
	if err != nil {
		return nil, fmt.Errorf("invalid createTime: %v", err)
	}

	deleteTime, err := ptypes.Timestamp(snapshot.DeleteTime)
	if err != nil {
		return nil, fmt.Errorf("invalid deleteTime: %v", err)
	}

	return &SnapshotInfo{
		Name:        name,
		SourceTable: tableID,
		DataSize:    snapshot.DataSizeBytes,
		CreateTime:  createTime,
		DeleteTime:  deleteTime,
	}, nil
}

// SnapshotIterator is an EntryIterator that iterates over log entries.
//
// This is a private alpha release of Cloud Bigtable snapshots. This feature
// is not currently available to most Cloud Bigtable customers. This feature
// might be changed in backward-incompatible ways and is not recommended for
// production use. It is not subject to any SLA or deprecation policy.
type SnapshotIterator struct {
	items    []*SnapshotInfo
	pageInfo *iterator.PageInfo
	nextFunc func() error
}

// PageInfo supports pagination. See https://godoc.org/google.golang.org/api/iterator package for details.
func (it *SnapshotIterator) PageInfo() *iterator.PageInfo {
	return it.pageInfo
}

// Next returns the next result. Its second return value is iterator.Done
// (https://godoc.org/google.golang.org/api/iterator) if there are no more
// results. Once Next returns Done, all subsequent calls will return Done.
func (it *SnapshotIterator) Next() (*SnapshotInfo, error) {
	if err := it.nextFunc(); err != nil {
		return nil, err
	}
	item := it.items[0]
	it.items = it.items[1:]
	return item, nil
}

// SnapshotInfo contains snapshot metadata.
type SnapshotInfo struct {
	Name        string
	SourceTable string
	DataSize    int64
	CreateTime  time.Time
	DeleteTime  time.Time
}

// SnapshotInfo gets snapshot metadata.
//
// This is a private alpha release of Cloud Bigtable snapshots. This feature
// is not currently available to most Cloud Bigtable customers. This feature
// might be changed in backward-incompatible ways and is not recommended for
// production use. It is not subject to any SLA or deprecation policy.
func (ac *AdminClient) SnapshotInfo(ctx context.Context, cluster, snapshot string) (*SnapshotInfo, error) {
	ctx = mergeOutgoingMetadata(ctx, ac.md)
	prefix := ac.instancePrefix()
	clusterPath := prefix + "/clusters/" + cluster
	snapshotPath := clusterPath + "/snapshots/" + snapshot

	req := &btapb.GetSnapshotRequest{
		Name: snapshotPath,
	}

	var resp *btapb.Snapshot
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		resp, err = ac.tClient.GetSnapshot(ctx, req)
		return err
	}, retryOptions...)
	if err != nil {
		return nil, err
	}

	return newSnapshotInfo(resp)
}

// DeleteSnapshot deletes a snapshot in a cluster.
//
// This is a private alpha release of Cloud Bigtable snapshots. This feature
// is not currently available to most Cloud Bigtable customers. This feature
// might be changed in backward-incompatible ways and is not recommended for
// production use. It is not subject to any SLA or deprecation policy.
func (ac *AdminClient) DeleteSnapshot(ctx context.Context, cluster, snapshot string) error {
	ctx = mergeOutgoingMetadata(ctx, ac.md)
	prefix := ac.instancePrefix()
	clusterPath := prefix + "/clusters/" + cluster
	snapshotPath := clusterPath + "/snapshots/" + snapshot

	req := &btapb.DeleteSnapshotRequest{
		Name: snapshotPath,
	}
	_, err := ac.tClient.DeleteSnapshot(ctx, req)
	return err
}

// getConsistencyToken gets the consistency token for a table.
func (ac *AdminClient) getConsistencyToken(ctx context.Context, tableName string) (string, error) {
	req := &btapb.GenerateConsistencyTokenRequest{
		Name: tableName,
	}
	resp, err := ac.tClient.GenerateConsistencyToken(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.GetConsistencyToken(), nil
}

// isConsistent checks if a token is consistent for a table.
func (ac *AdminClient) isConsistent(ctx context.Context, tableName, token string) (bool, error) {
	req := &btapb.CheckConsistencyRequest{
		Name:             tableName,
		ConsistencyToken: token,
	}
	var resp *btapb.CheckConsistencyResponse

	// Retry calls on retryable errors to avoid losing the token gathered before.
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		resp, err = ac.tClient.CheckConsistency(ctx, req)
		return err
	}, retryOptions...)
	if err != nil {
		return false, err
	}
	return resp.GetConsistent(), nil
}

// WaitForReplication waits until all the writes committed before the call started have been propagated to all the clusters in the instance via replication.
func (ac *AdminClient) WaitForReplication(ctx context.Context, table string) error {
	// Get the token.
	prefix := ac.instancePrefix()
	tableName := prefix + "/tables/" + table
	token, err := ac.getConsistencyToken(ctx, tableName)
	if err != nil {
		return err
	}

	// Periodically check if the token is consistent.
	timer := time.NewTicker(time.Second * 10)
	defer timer.Stop()
	for {
		consistent, err := ac.isConsistent(ctx, tableName, token)
		if err != nil {
			return err
		}
		if consistent {
			return nil
		}
		// Sleep for a bit or until the ctx is cancelled.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
}

const instanceAdminAddr = "bigtableadmin.googleapis.com:443"

// InstanceAdminClient is a client type for performing admin operations on instances.
// These operations can be substantially more dangerous than those provided by AdminClient.
type InstanceAdminClient struct {
	conn      *grpc.ClientConn
	iClient   btapb.BigtableInstanceAdminClient
	lroClient *lroauto.OperationsClient

	project string

	// Metadata to be sent with each request.
	md metadata.MD
}

// NewInstanceAdminClient creates a new InstanceAdminClient for a given project.
func NewInstanceAdminClient(ctx context.Context, project string, opts ...option.ClientOption) (*InstanceAdminClient, error) {
	o, err := btopt.DefaultClientOptions(instanceAdminAddr, InstanceAdminScope, clientUserAgent)
	if err != nil {
		return nil, err
	}
	o = append(o, opts...)
	conn, err := gtransport.Dial(ctx, o...)
	if err != nil {
		return nil, fmt.Errorf("dialing: %v", err)
	}

	lroClient, err := lroauto.NewOperationsClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		// This error "should not happen", since we are just reusing old connection
		// and never actually need to dial.
		// If this does happen, we could leak conn. However, we cannot close conn:
		// If the user invoked the function with option.WithGRPCConn,
		// we would close a connection that's still in use.
		// TODO(pongad): investigate error conditions.
		return nil, err
	}

	return &InstanceAdminClient{
		conn:      conn,
		iClient:   btapb.NewBigtableInstanceAdminClient(conn),
		lroClient: lroClient,

		project: project,
		md:      metadata.Pairs(resourcePrefixHeader, "projects/"+project),
	}, nil
}

// Close closes the InstanceAdminClient.
func (iac *InstanceAdminClient) Close() error {
	return iac.conn.Close()
}

// StorageType is the type of storage used for all tables in an instance
type StorageType int

const (
	SSD StorageType = iota
	HDD
)

func (st StorageType) proto() btapb.StorageType {
	if st == HDD {
		return btapb.StorageType_HDD
	}
	return btapb.StorageType_SSD
}

// InstanceType is the type of the instance
type InstanceType int32

const (
	PRODUCTION  InstanceType = InstanceType(btapb.Instance_PRODUCTION)
	DEVELOPMENT              = InstanceType(btapb.Instance_DEVELOPMENT)
)

// InstanceInfo represents information about an instance
type InstanceInfo struct {
	Name        string // name of the instance
	DisplayName string // display name for UIs
}

// InstanceConf contains the information necessary to create an Instance
type InstanceConf struct {
	InstanceId, DisplayName, ClusterId, Zone string
	// NumNodes must not be specified for DEVELOPMENT instance types
	NumNodes     int32
	StorageType  StorageType
	InstanceType InstanceType
}

// InstanceWithClustersConfig contains the information necessary to create an Instance
type InstanceWithClustersConfig struct {
	InstanceID, DisplayName string
	Clusters                []ClusterConfig
	InstanceType            InstanceType
}

var instanceNameRegexp = regexp.MustCompile(`^projects/([^/]+)/instances/([a-z][-a-z0-9]*)$`)

// CreateInstance creates a new instance in the project.
// This method will return when the instance has been created or when an error occurs.
func (iac *InstanceAdminClient) CreateInstance(ctx context.Context, conf *InstanceConf) error {
	newConfig := InstanceWithClustersConfig{
		InstanceID:   conf.InstanceId,
		DisplayName:  conf.DisplayName,
		InstanceType: conf.InstanceType,
		Clusters: []ClusterConfig{
			{
				InstanceID:  conf.InstanceId,
				ClusterID:   conf.ClusterId,
				Zone:        conf.Zone,
				NumNodes:    conf.NumNodes,
				StorageType: conf.StorageType,
			},
		},
	}
	return iac.CreateInstanceWithClusters(ctx, &newConfig)
}

// CreateInstanceWithClusters creates a new instance with configured clusters in the project.
// This method will return when the instance has been created or when an error occurs.
func (iac *InstanceAdminClient) CreateInstanceWithClusters(ctx context.Context, conf *InstanceWithClustersConfig) error {
	ctx = mergeOutgoingMetadata(ctx, iac.md)
	clusters := make(map[string]*btapb.Cluster)
	for _, cluster := range conf.Clusters {
		clusters[cluster.ClusterID] = cluster.proto(iac.project)
	}

	req := &btapb.CreateInstanceRequest{
		Parent:     "projects/" + iac.project,
		InstanceId: conf.InstanceID,
		Instance:   &btapb.Instance{DisplayName: conf.DisplayName, Type: btapb.Instance_Type(conf.InstanceType)},
		Clusters:   clusters,
	}

	lro, err := iac.iClient.CreateInstance(ctx, req)
	if err != nil {
		return err
	}
	resp := btapb.Instance{}
	return longrunning.InternalNewOperation(iac.lroClient, lro).Wait(ctx, &resp)
}

// DeleteInstance deletes an instance from the project.
func (iac *InstanceAdminClient) DeleteInstance(ctx context.Context, instanceID string) error {
	ctx = mergeOutgoingMetadata(ctx, iac.md)
	req := &btapb.DeleteInstanceRequest{Name: "projects/" + iac.project + "/instances/" + instanceID}
	_, err := iac.iClient.DeleteInstance(ctx, req)
	return err
}

// Instances returns a list of instances in the project.
func (iac *InstanceAdminClient) Instances(ctx context.Context) ([]*InstanceInfo, error) {
	ctx = mergeOutgoingMetadata(ctx, iac.md)
	req := &btapb.ListInstancesRequest{
		Parent: "projects/" + iac.project,
	}
	var res *btapb.ListInstancesResponse
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		res, err = iac.iClient.ListInstances(ctx, req)
		return err
	}, retryOptions...)
	if err != nil {
		return nil, err
	}
	if len(res.FailedLocations) > 0 {
		// We don't have a good way to return a partial result in the face of some zones being unavailable.
		// Fail the entire request.
		return nil, status.Errorf(codes.Unavailable, "Failed locations: %v", res.FailedLocations)
	}

	var is []*InstanceInfo
	for _, i := range res.Instances {
		m := instanceNameRegexp.FindStringSubmatch(i.Name)
		if m == nil {
			return nil, fmt.Errorf("malformed instance name %q", i.Name)
		}
		is = append(is, &InstanceInfo{
			Name:        m[2],
			DisplayName: i.DisplayName,
		})
	}
	return is, nil
}

// InstanceInfo returns information about an instance.
func (iac *InstanceAdminClient) InstanceInfo(ctx context.Context, instanceID string) (*InstanceInfo, error) {
	ctx = mergeOutgoingMetadata(ctx, iac.md)
	req := &btapb.GetInstanceRequest{
		Name: "projects/" + iac.project + "/instances/" + instanceID,
	}
	var res *btapb.Instance
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		res, err = iac.iClient.GetInstance(ctx, req)
		return err
	}, retryOptions...)
	if err != nil {
		return nil, err
	}

	m := instanceNameRegexp.FindStringSubmatch(res.Name)
	if m == nil {
		return nil, fmt.Errorf("malformed instance name %q", res.Name)
	}
	return &InstanceInfo{
		Name:        m[2],
		DisplayName: res.DisplayName,
	}, nil
}

// ClusterConfig contains the information necessary to create a cluster
type ClusterConfig struct {
	InstanceID, ClusterID, Zone string
	NumNodes                    int32
	StorageType                 StorageType
}

func (cc *ClusterConfig) proto(project string) *btapb.Cluster {
	return &btapb.Cluster{
		ServeNodes:         cc.NumNodes,
		DefaultStorageType: cc.StorageType.proto(),
		Location:           "projects/" + project + "/locations/" + cc.Zone,
	}
}

// ClusterInfo represents information about a cluster.
type ClusterInfo struct {
	Name       string // name of the cluster
	Zone       string // GCP zone of the cluster (e.g. "us-central1-a")
	ServeNodes int    // number of allocated serve nodes
	State      string // state of the cluster
}

// CreateCluster creates a new cluster in an instance.
// This method will return when the cluster has been created or when an error occurs.
func (iac *InstanceAdminClient) CreateCluster(ctx context.Context, conf *ClusterConfig) error {
	ctx = mergeOutgoingMetadata(ctx, iac.md)

	req := &btapb.CreateClusterRequest{
		Parent:    "projects/" + iac.project + "/instances/" + conf.InstanceID,
		ClusterId: conf.ClusterID,
		Cluster:   conf.proto(iac.project),
	}

	lro, err := iac.iClient.CreateCluster(ctx, req)
	if err != nil {
		return err
	}
	resp := btapb.Cluster{}
	return longrunning.InternalNewOperation(iac.lroClient, lro).Wait(ctx, &resp)
}

// DeleteCluster deletes a cluster from an instance.
func (iac *InstanceAdminClient) DeleteCluster(ctx context.Context, instanceID, clusterID string) error {
	ctx = mergeOutgoingMetadata(ctx, iac.md)
	req := &btapb.DeleteClusterRequest{Name: "projects/" + iac.project + "/instances/" + instanceID + "/clusters/" + clusterID}
	_, err := iac.iClient.DeleteCluster(ctx, req)
	return err
}

// UpdateCluster updates attributes of a cluster
func (iac *InstanceAdminClient) UpdateCluster(ctx context.Context, instanceID, clusterID string, serveNodes int32) error {
	ctx = mergeOutgoingMetadata(ctx, iac.md)
	cluster := &btapb.Cluster{
		Name:       "projects/" + iac.project + "/instances/" + instanceID + "/clusters/" + clusterID,
		ServeNodes: serveNodes}
	lro, err := iac.iClient.UpdateCluster(ctx, cluster)
	if err != nil {
		return err
	}
	return longrunning.InternalNewOperation(iac.lroClient, lro).Wait(ctx, nil)
}

// Clusters lists the clusters in an instance.
func (iac *InstanceAdminClient) Clusters(ctx context.Context, instanceID string) ([]*ClusterInfo, error) {
	ctx = mergeOutgoingMetadata(ctx, iac.md)
	req := &btapb.ListClustersRequest{Parent: "projects/" + iac.project + "/instances/" + instanceID}
	var res *btapb.ListClustersResponse
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		res, err = iac.iClient.ListClusters(ctx, req)
		return err
	}, retryOptions...)
	if err != nil {
		return nil, err
	}
	// TODO(garyelliott): Deal with failed_locations.
	var cis []*ClusterInfo
	for _, c := range res.Clusters {
		nameParts := strings.Split(c.Name, "/")
		locParts := strings.Split(c.Location, "/")
		cis = append(cis, &ClusterInfo{
			Name:       nameParts[len(nameParts)-1],
			Zone:       locParts[len(locParts)-1],
			ServeNodes: int(c.ServeNodes),
			State:      c.State.String(),
		})
	}
	return cis, nil
}

// GetCluster fetches a cluster in an instance
func (iac *InstanceAdminClient) GetCluster(ctx context.Context, instanceID, clusterID string) (*ClusterInfo, error) {
	ctx = mergeOutgoingMetadata(ctx, iac.md)
	req := &btapb.GetClusterRequest{Name: "projects/" + iac.project + "/instances/" + instanceID + "/clusters/" + clusterID}
	var c *btapb.Cluster
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		c, err = iac.iClient.GetCluster(ctx, req)
		return err
	}, retryOptions...)
	if err != nil {
		return nil, err
	}

	nameParts := strings.Split(c.Name, "/")
	locParts := strings.Split(c.Location, "/")
	cis := &ClusterInfo{
		Name:       nameParts[len(nameParts)-1],
		Zone:       locParts[len(locParts)-1],
		ServeNodes: int(c.ServeNodes),
		State:      c.State.String(),
	}
	return cis, nil
}

// InstanceIAM returns the instance's IAM handle.
func (iac *InstanceAdminClient) InstanceIAM(instanceID string) *iam.Handle {
	return iam.InternalNewHandleGRPCClient(iac.iClient, "projects/"+iac.project+"/instances/"+instanceID)

}

// Routing policies.
const (
	// MultiClusterRouting is a policy that allows read/write requests to be
	// routed to any cluster in the instance. Requests will will fail over to
	// another cluster in the event of transient errors or delays. Choosing
	// this option sacrifices read-your-writes consistency to improve
	// availability.
	MultiClusterRouting = "multi_cluster_routing_use_any"
	// SingleClusterRouting is a policy that unconditionally routes all
	// read/write requests to a specific cluster. This option preserves
	// read-your-writes consistency, but does not improve availability.
	SingleClusterRouting = "single_cluster_routing"
)

// ProfileConf contains the information necessary to create an profile
type ProfileConf struct {
	Name                     string
	ProfileID                string
	InstanceID               string
	Etag                     string
	Description              string
	RoutingPolicy            string
	ClusterID                string
	AllowTransactionalWrites bool

	// If true, warnings are ignored
	IgnoreWarnings bool
}

// ProfileIterator iterates over profiles.
type ProfileIterator struct {
	items    []*btapb.AppProfile
	pageInfo *iterator.PageInfo
	nextFunc func() error
}

// ProfileAttrsToUpdate define addrs to update during an Update call. If unset, no fields will be replaced.
type ProfileAttrsToUpdate struct {
	// If set, updates the description.
	Description optional.String

	//If set, updates the routing policy.
	RoutingPolicy optional.String

	//If RoutingPolicy is updated to SingleClusterRouting, set these fields as well.
	ClusterID                string
	AllowTransactionalWrites bool

	// If true, warnings are ignored
	IgnoreWarnings bool
}

// GetFieldMaskPath returns the field mask path.
func (p *ProfileAttrsToUpdate) GetFieldMaskPath() []string {
	path := make([]string, 0)
	if p.Description != nil {
		path = append(path, "description")
	}

	if p.RoutingPolicy != nil {
		path = append(path, optional.ToString(p.RoutingPolicy))
	}
	return path
}

// PageInfo supports pagination. See https://godoc.org/google.golang.org/api/iterator package for details.
func (it *ProfileIterator) PageInfo() *iterator.PageInfo {
	return it.pageInfo
}

// Next returns the next result. Its second return value is iterator.Done
// (https://godoc.org/google.golang.org/api/iterator) if there are no more
// results. Once Next returns Done, all subsequent calls will return Done.
func (it *ProfileIterator) Next() (*btapb.AppProfile, error) {
	if err := it.nextFunc(); err != nil {
		return nil, err
	}
	item := it.items[0]
	it.items = it.items[1:]
	return item, nil
}

// CreateAppProfile creates an app profile within an instance.
func (iac *InstanceAdminClient) CreateAppProfile(ctx context.Context, profile ProfileConf) (*btapb.AppProfile, error) {
	ctx = mergeOutgoingMetadata(ctx, iac.md)
	parent := "projects/" + iac.project + "/instances/" + profile.InstanceID
	appProfile := &btapb.AppProfile{
		Etag:        profile.Etag,
		Description: profile.Description,
	}

	if profile.RoutingPolicy == "" {
		return nil, errors.New("invalid routing policy")
	}

	switch profile.RoutingPolicy {
	case MultiClusterRouting:
		appProfile.RoutingPolicy = &btapb.AppProfile_MultiClusterRoutingUseAny_{
			MultiClusterRoutingUseAny: &btapb.AppProfile_MultiClusterRoutingUseAny{},
		}
	case SingleClusterRouting:
		appProfile.RoutingPolicy = &btapb.AppProfile_SingleClusterRouting_{
			SingleClusterRouting: &btapb.AppProfile_SingleClusterRouting{
				ClusterId:                profile.ClusterID,
				AllowTransactionalWrites: profile.AllowTransactionalWrites,
			},
		}
	default:
		return nil, errors.New("invalid routing policy")
	}

	return iac.iClient.CreateAppProfile(ctx, &btapb.CreateAppProfileRequest{
		Parent:         parent,
		AppProfile:     appProfile,
		AppProfileId:   profile.ProfileID,
		IgnoreWarnings: profile.IgnoreWarnings,
	})
}

// GetAppProfile gets information about an app profile.
func (iac *InstanceAdminClient) GetAppProfile(ctx context.Context, instanceID, name string) (*btapb.AppProfile, error) {
	ctx = mergeOutgoingMetadata(ctx, iac.md)
	profileRequest := &btapb.GetAppProfileRequest{
		Name: "projects/" + iac.project + "/instances/" + instanceID + "/appProfiles/" + name,
	}
	var ap *btapb.AppProfile
	err := gax.Invoke(ctx, func(ctx context.Context) error {
		var err error
		ap, err = iac.iClient.GetAppProfile(ctx, profileRequest)
		return err
	}, retryOptions...)
	if err != nil {
		return nil, err
	}
	return ap, err
}

// ListAppProfiles lists information about app profiles in an instance.
func (iac *InstanceAdminClient) ListAppProfiles(ctx context.Context, instanceID string) *ProfileIterator {
	ctx = mergeOutgoingMetadata(ctx, iac.md)
	listRequest := &btapb.ListAppProfilesRequest{
		Parent: "projects/" + iac.project + "/instances/" + instanceID,
	}

	pit := &ProfileIterator{}
	fetch := func(pageSize int, pageToken string) (string, error) {
		listRequest.PageToken = pageToken
		var profileRes *btapb.ListAppProfilesResponse
		err := gax.Invoke(ctx, func(ctx context.Context) error {
			var err error
			profileRes, err = iac.iClient.ListAppProfiles(ctx, listRequest)
			return err
		}, retryOptions...)
		if err != nil {
			return "", err
		}

		pit.items = append(pit.items, profileRes.AppProfiles...)
		return profileRes.NextPageToken, nil
	}

	bufLen := func() int { return len(pit.items) }
	takeBuf := func() interface{} { b := pit.items; pit.items = nil; return b }
	pit.pageInfo, pit.nextFunc = iterator.NewPageInfo(fetch, bufLen, takeBuf)
	return pit

}

// UpdateAppProfile updates an app profile within an instance.
// updateAttrs should be set. If unset, all fields will be replaced.
func (iac *InstanceAdminClient) UpdateAppProfile(ctx context.Context, instanceID, profileID string, updateAttrs ProfileAttrsToUpdate) error {
	ctx = mergeOutgoingMetadata(ctx, iac.md)

	profile := &btapb.AppProfile{
		Name: "projects/" + iac.project + "/instances/" + instanceID + "/appProfiles/" + profileID,
	}

	if updateAttrs.Description != nil {
		profile.Description = optional.ToString(updateAttrs.Description)
	}
	if updateAttrs.RoutingPolicy != nil {
		switch optional.ToString(updateAttrs.RoutingPolicy) {
		case MultiClusterRouting:
			profile.RoutingPolicy = &btapb.AppProfile_MultiClusterRoutingUseAny_{
				MultiClusterRoutingUseAny: &btapb.AppProfile_MultiClusterRoutingUseAny{},
			}
		case SingleClusterRouting:
			profile.RoutingPolicy = &btapb.AppProfile_SingleClusterRouting_{
				SingleClusterRouting: &btapb.AppProfile_SingleClusterRouting{
					ClusterId:                updateAttrs.ClusterID,
					AllowTransactionalWrites: updateAttrs.AllowTransactionalWrites,
				},
			}
		default:
			return errors.New("invalid routing policy")
		}
	}
	patchRequest := &btapb.UpdateAppProfileRequest{
		AppProfile: profile,
		UpdateMask: &field_mask.FieldMask{
			Paths: updateAttrs.GetFieldMaskPath(),
		},
		IgnoreWarnings: updateAttrs.IgnoreWarnings,
	}
	updateRequest, err := iac.iClient.UpdateAppProfile(ctx, patchRequest)
	if err != nil {
		return err
	}

	return longrunning.InternalNewOperation(iac.lroClient, updateRequest).Wait(ctx, nil)

}

// DeleteAppProfile deletes an app profile from an instance.
func (iac *InstanceAdminClient) DeleteAppProfile(ctx context.Context, instanceID, name string) error {
	ctx = mergeOutgoingMetadata(ctx, iac.md)
	deleteProfileRequest := &btapb.DeleteAppProfileRequest{
		Name:           "projects/" + iac.project + "/instances/" + instanceID + "/appProfiles/" + name,
		IgnoreWarnings: true,
	}
	_, err := iac.iClient.DeleteAppProfile(ctx, deleteProfileRequest)
	return err

}
