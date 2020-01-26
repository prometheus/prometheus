// Copyright 2017 Google LLC
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

package firestore

// A simple mock server.

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"cloud.google.com/go/internal/testutil"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockServer struct {
	pb.FirestoreServer

	Addr string

	reqItems []reqItem
	resps    []interface{}
}

type reqItem struct {
	wantReq proto.Message
	adjust  func(gotReq proto.Message)
}

func newMockServer() (*mockServer, error) {
	srv, err := testutil.NewServer()
	if err != nil {
		return nil, err
	}
	mock := &mockServer{Addr: srv.Addr}
	pb.RegisterFirestoreServer(srv.Gsrv, mock)
	srv.Start()
	return mock, nil
}

// addRPC adds a (request, response) pair to the server's list of expected
// interactions. The server will compare the incoming request with wantReq
// using proto.Equal. The response can be a message or an error.
//
// For the Listen RPC, resp should be a []interface{}, where each element
// is either ListenResponse or an error.
//
// Passing nil for wantReq disables the request check.
func (s *mockServer) addRPC(wantReq proto.Message, resp interface{}) {
	s.addRPCAdjust(wantReq, resp, nil)
}

// addRPCAdjust is like addRPC, but accepts a function that can be used
// to tweak the requests before comparison, for example to adjust for
// randomness.
func (s *mockServer) addRPCAdjust(wantReq proto.Message, resp interface{}, adjust func(proto.Message)) {
	s.reqItems = append(s.reqItems, reqItem{wantReq, adjust})
	s.resps = append(s.resps, resp)
}

// popRPC compares the request with the next expected (request, response) pair.
// It returns the response, or an error if the request doesn't match what
// was expected or there are no expected rpcs.
func (s *mockServer) popRPC(gotReq proto.Message) (interface{}, error) {
	if len(s.reqItems) == 0 {
		panic(fmt.Sprintf("out of RPCs, saw %v", reflect.TypeOf(gotReq)))
	}
	ri := s.reqItems[0]
	s.reqItems = s.reqItems[1:]
	if ri.wantReq != nil {
		if ri.adjust != nil {
			ri.adjust(gotReq)
		}

		// Sort FieldTransforms by FieldPath, since slice order is undefined and proto.Equal
		// is strict about order.
		switch gotReqTyped := gotReq.(type) {
		case *pb.CommitRequest:
			for _, w := range gotReqTyped.Writes {
				switch opTyped := w.Operation.(type) {
				case *pb.Write_Transform:
					sort.Sort(ByFieldPath(opTyped.Transform.FieldTransforms))
				}
			}
		}

		if !proto.Equal(gotReq, ri.wantReq) {
			return nil, fmt.Errorf("mockServer: bad request\ngot:\n%T\n%s\nwant:\n%T\n%s",
				gotReq, proto.MarshalTextString(gotReq),
				ri.wantReq, proto.MarshalTextString(ri.wantReq))
		}
	}
	resp := s.resps[0]
	s.resps = s.resps[1:]
	if err, ok := resp.(error); ok {
		return nil, err
	}
	return resp, nil
}

func (a ByFieldPath) Len() int           { return len(a) }
func (a ByFieldPath) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByFieldPath) Less(i, j int) bool { return a[i].FieldPath < a[j].FieldPath }

type ByFieldPath []*pb.DocumentTransform_FieldTransform

func (s *mockServer) reset() {
	s.reqItems = nil
	s.resps = nil
}

func (s *mockServer) GetDocument(_ context.Context, req *pb.GetDocumentRequest) (*pb.Document, error) {
	res, err := s.popRPC(req)
	if err != nil {
		return nil, err
	}
	return res.(*pb.Document), nil
}

func (s *mockServer) Commit(_ context.Context, req *pb.CommitRequest) (*pb.CommitResponse, error) {
	res, err := s.popRPC(req)
	if err != nil {
		return nil, err
	}
	return res.(*pb.CommitResponse), nil
}

func (s *mockServer) BatchGetDocuments(req *pb.BatchGetDocumentsRequest, bs pb.Firestore_BatchGetDocumentsServer) error {
	res, err := s.popRPC(req)
	if err != nil {
		return err
	}
	responses := res.([]interface{})
	for _, res := range responses {
		switch res := res.(type) {
		case *pb.BatchGetDocumentsResponse:
			if err := bs.Send(res); err != nil {
				return err
			}
		case error:
			return res
		default:
			panic(fmt.Sprintf("bad response type in BatchGetDocuments: %+v", res))
		}
	}
	return nil
}

func (s *mockServer) RunQuery(req *pb.RunQueryRequest, qs pb.Firestore_RunQueryServer) error {
	res, err := s.popRPC(req)
	if err != nil {
		return err
	}
	responses := res.([]interface{})
	for _, res := range responses {
		switch res := res.(type) {
		case *pb.RunQueryResponse:
			if err := qs.Send(res); err != nil {
				return err
			}
		case error:
			return res
		default:
			panic(fmt.Sprintf("bad response type in RunQuery: %+v", res))
		}
	}
	return nil
}

func (s *mockServer) BeginTransaction(_ context.Context, req *pb.BeginTransactionRequest) (*pb.BeginTransactionResponse, error) {
	res, err := s.popRPC(req)
	if err != nil {
		return nil, err
	}
	return res.(*pb.BeginTransactionResponse), nil
}

func (s *mockServer) Rollback(_ context.Context, req *pb.RollbackRequest) (*empty.Empty, error) {
	res, err := s.popRPC(req)
	if err != nil {
		return nil, err
	}
	return res.(*empty.Empty), nil
}

func (s *mockServer) Listen(stream pb.Firestore_ListenServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}
	responses, err := s.popRPC(req)
	if err != nil {
		if status.Code(err) == codes.Unknown && strings.Contains(err.Error(), "mockServer") {
			// The stream will retry on Unknown, but we don't want that to happen if
			// the error comes from us.
			panic(err)
		}
		return err
	}
	for _, res := range responses.([]interface{}) {
		if err, ok := res.(error); ok {
			return err
		}
		if err := stream.Send(res.(*pb.ListenResponse)); err != nil {
			return err
		}
	}
	return nil
}
