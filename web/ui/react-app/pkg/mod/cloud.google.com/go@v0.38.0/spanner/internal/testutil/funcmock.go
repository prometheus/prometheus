/*
Copyright 2018 Google LLC

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

package testutil

import (
	"context"

	sppb "google.golang.org/genproto/googleapis/spanner/v1"
	"google.golang.org/grpc"
)

// FuncMock overloads some of MockCloudSpannerClient's methods with pluggable
// functions.
//
// Note: if you overload a method, you're in charge of making sure
// MockCloudSpannerClient.ReceivedRequests receives the request appropriately.
type FuncMock struct {
	CommitFn           func(ctx context.Context, r *sppb.CommitRequest, opts ...grpc.CallOption) (*sppb.CommitResponse, error)
	BeginTransactionFn func(ctx context.Context, r *sppb.BeginTransactionRequest, opts ...grpc.CallOption) (*sppb.Transaction, error)
	GetSessionFn       func(ctx context.Context, r *sppb.GetSessionRequest, opts ...grpc.CallOption) (*sppb.Session, error)
	CreateSessionFn    func(ctx context.Context, r *sppb.CreateSessionRequest, opts ...grpc.CallOption) (*sppb.Session, error)
	*MockCloudSpannerClient
}

func (s FuncMock) Commit(ctx context.Context, r *sppb.CommitRequest, opts ...grpc.CallOption) (*sppb.CommitResponse, error) {
	if s.CommitFn == nil {
		return s.MockCloudSpannerClient.Commit(ctx, r, opts...)
	}
	return s.CommitFn(ctx, r, opts...)
}

func (s FuncMock) BeginTransaction(ctx context.Context, r *sppb.BeginTransactionRequest, opts ...grpc.CallOption) (*sppb.Transaction, error) {
	if s.BeginTransactionFn == nil {
		return s.MockCloudSpannerClient.BeginTransaction(ctx, r, opts...)
	}
	return s.BeginTransactionFn(ctx, r, opts...)
}

func (s *FuncMock) GetSession(ctx context.Context, r *sppb.GetSessionRequest, opts ...grpc.CallOption) (*sppb.Session, error) {
	if s.GetSessionFn == nil {
		return s.MockCloudSpannerClient.GetSession(ctx, r, opts...)
	}
	return s.GetSessionFn(ctx, r, opts...)
}

func (s *FuncMock) CreateSession(c context.Context, r *sppb.CreateSessionRequest, opts ...grpc.CallOption) (*sppb.Session, error) {
	if s.CreateSessionFn == nil {
		return s.MockCloudSpannerClient.CreateSession(c, r, opts...)
	}
	return s.CreateSessionFn(c, r, opts...)
}
