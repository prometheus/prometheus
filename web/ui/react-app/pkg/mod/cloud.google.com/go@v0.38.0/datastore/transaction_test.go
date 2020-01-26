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

package datastore

import (
	"context"
	"testing"

	"github.com/golang/protobuf/proto"
	pb "google.golang.org/genproto/googleapis/datastore/v1"
)

func TestNewTransaction(t *testing.T) {
	var got *pb.BeginTransactionRequest
	client := &Client{
		dataset: "project",
		client: &fakeDatastoreClient{
			beginTransaction: func(req *pb.BeginTransactionRequest) (*pb.BeginTransactionResponse, error) {
				got = req
				return &pb.BeginTransactionResponse{
					Transaction: []byte("tid"),
				}, nil
			},
		},
	}
	ctx := context.Background()
	for _, test := range []struct {
		settings *transactionSettings
		want     *pb.BeginTransactionRequest
	}{
		{
			&transactionSettings{},
			&pb.BeginTransactionRequest{ProjectId: "project"},
		},
		{
			&transactionSettings{readOnly: true},
			&pb.BeginTransactionRequest{
				ProjectId: "project",
				TransactionOptions: &pb.TransactionOptions{
					Mode: &pb.TransactionOptions_ReadOnly_{ReadOnly: &pb.TransactionOptions_ReadOnly{}},
				},
			},
		},
		{
			&transactionSettings{prevID: []byte("tid")},
			&pb.BeginTransactionRequest{
				ProjectId: "project",
				TransactionOptions: &pb.TransactionOptions{
					Mode: &pb.TransactionOptions_ReadWrite_{ReadWrite: &pb.TransactionOptions_ReadWrite{
						PreviousTransaction: []byte("tid"),
					},
					},
				},
			},
		},
	} {
		_, err := client.newTransaction(ctx, test.settings)
		if err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(got, test.want) {
			t.Errorf("%+v:\ngot  %+v\nwant %+v", test.settings, got, test.want)
		}
	}
}
