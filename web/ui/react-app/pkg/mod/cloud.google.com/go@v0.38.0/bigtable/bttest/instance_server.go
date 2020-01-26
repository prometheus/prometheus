// Copyright 2019 Google LLC
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

package bttest

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	btapb "google.golang.org/genproto/googleapis/bigtable/admin/v2"
	iampb "google.golang.org/genproto/googleapis/iam/v1"
	"google.golang.org/genproto/googleapis/longrunning"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ btapb.BigtableInstanceAdminServer = (*server)(nil)

var errUnimplemented = status.Error(codes.Unimplemented, "unimplemented feature")

func (s *server) CreateInstance(ctx context.Context, req *btapb.CreateInstanceRequest) (*longrunning.Operation, error) {
	return nil, errUnimplemented
}

func (s *server) GetInstance(ctx context.Context, req *btapb.GetInstanceRequest) (*btapb.Instance, error) {
	return nil, errUnimplemented
}

func (s *server) ListInstances(ctx context.Context, req *btapb.ListInstancesRequest) (*btapb.ListInstancesResponse, error) {
	return nil, errUnimplemented
}

func (s *server) UpdateInstance(ctx context.Context, req *btapb.Instance) (*btapb.Instance, error) {
	return nil, errUnimplemented
}

func (s *server) PartialUpdateInstance(ctx context.Context, req *btapb.PartialUpdateInstanceRequest) (*longrunning.Operation, error) {
	return nil, errUnimplemented
}

func (s *server) DeleteInstance(ctx context.Context, req *btapb.DeleteInstanceRequest) (*empty.Empty, error) {
	return nil, errUnimplemented
}

func (s *server) CreateCluster(ctx context.Context, req *btapb.CreateClusterRequest) (*longrunning.Operation, error) {
	return nil, errUnimplemented
}

func (s *server) GetCluster(ctx context.Context, req *btapb.GetClusterRequest) (*btapb.Cluster, error) {
	return nil, errUnimplemented
}

func (s *server) ListClusters(ctx context.Context, req *btapb.ListClustersRequest) (*btapb.ListClustersResponse, error) {
	return nil, errUnimplemented
}

func (s *server) UpdateCluster(ctx context.Context, req *btapb.Cluster) (*longrunning.Operation, error) {
	return nil, errUnimplemented
}

func (s *server) DeleteCluster(ctx context.Context, req *btapb.DeleteClusterRequest) (*empty.Empty, error) {
	return nil, errUnimplemented
}

func (s *server) CreateAppProfile(ctx context.Context, req *btapb.CreateAppProfileRequest) (*btapb.AppProfile, error) {
	return nil, errUnimplemented
}

func (s *server) GetAppProfile(ctx context.Context, req *btapb.GetAppProfileRequest) (*btapb.AppProfile, error) {
	return nil, errUnimplemented
}

func (s *server) ListAppProfiles(ctx context.Context, req *btapb.ListAppProfilesRequest) (*btapb.ListAppProfilesResponse, error) {
	return nil, errUnimplemented
}

func (s *server) UpdateAppProfile(ctx context.Context, req *btapb.UpdateAppProfileRequest) (*longrunning.Operation, error) {
	return nil, errUnimplemented
}

func (s *server) DeleteAppProfile(ctx context.Context, req *btapb.DeleteAppProfileRequest) (*empty.Empty, error) {
	return nil, errUnimplemented
}

func (s *server) GetIamPolicy(ctx context.Context, req *iampb.GetIamPolicyRequest) (*iampb.Policy, error) {
	return nil, errUnimplemented
}

func (s *server) SetIamPolicy(ctx context.Context, req *iampb.SetIamPolicyRequest) (*iampb.Policy, error) {
	return nil, errUnimplemented
}

func (s *server) TestIamPermissions(ctx context.Context, req *iampb.TestIamPermissionsRequest) (*iampb.TestIamPermissionsResponse, error) {
	return nil, errUnimplemented
}
