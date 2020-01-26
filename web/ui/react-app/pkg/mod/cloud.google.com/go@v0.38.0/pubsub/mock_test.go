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

package pubsub

// This file provides a mock in-memory pubsub server for streaming pull testing.

import (
	"context"
	"io"
	"sync"
	"time"

	"cloud.google.com/go/internal/testutil"
	emptypb "github.com/golang/protobuf/ptypes/empty"
	pb "google.golang.org/genproto/googleapis/pubsub/v1"
)

type mockServer struct {
	srv *testutil.Server

	pb.SubscriberServer

	Addr string

	mu            sync.Mutex
	Acked         map[string]bool  // acked message IDs
	Deadlines     map[string]int32 // deadlines by message ID
	pullResponses []*pullResponse
	ackErrs       []error
	modAckErrs    []error
	wg            sync.WaitGroup
	sub           *pb.Subscription
}

type pullResponse struct {
	msgs []*pb.ReceivedMessage
	err  error
}

func newMockServer(port int) (*mockServer, error) {
	srv, err := testutil.NewServerWithPort(port)
	if err != nil {
		return nil, err
	}
	mock := &mockServer{
		srv:       srv,
		Addr:      srv.Addr,
		Acked:     map[string]bool{},
		Deadlines: map[string]int32{},
		sub: &pb.Subscription{
			AckDeadlineSeconds: 10,
			PushConfig:         &pb.PushConfig{},
		},
	}
	pb.RegisterSubscriberServer(srv.Gsrv, mock)
	srv.Start()
	return mock, nil
}

// Each call to addStreamingPullMessages results in one StreamingPullResponse.
func (s *mockServer) addStreamingPullMessages(msgs []*pb.ReceivedMessage) {
	s.mu.Lock()
	s.pullResponses = append(s.pullResponses, &pullResponse{msgs, nil})
	s.mu.Unlock()
}

func (s *mockServer) addStreamingPullError(err error) {
	s.mu.Lock()
	s.pullResponses = append(s.pullResponses, &pullResponse{nil, err})
	s.mu.Unlock()
}

func (s *mockServer) addAckResponse(err error) {
	s.mu.Lock()
	s.ackErrs = append(s.ackErrs, err)
	s.mu.Unlock()
}

func (s *mockServer) addModAckResponse(err error) {
	s.mu.Lock()
	s.modAckErrs = append(s.modAckErrs, err)
	s.mu.Unlock()
}

func (s *mockServer) wait() {
	s.wg.Wait()
}

func (s *mockServer) StreamingPull(stream pb.Subscriber_StreamingPullServer) error {
	s.wg.Add(1)
	defer s.wg.Done()
	errc := make(chan error, 1)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			req, err := stream.Recv()
			if err != nil {
				errc <- err
				return
			}
			s.mu.Lock()
			for _, id := range req.AckIds {
				s.Acked[id] = true
			}
			for i, id := range req.ModifyDeadlineAckIds {
				s.Deadlines[id] = req.ModifyDeadlineSeconds[i]
			}
			s.mu.Unlock()
		}
	}()
	// Send responses.
	for {
		s.mu.Lock()
		if len(s.pullResponses) == 0 {
			s.mu.Unlock()
			// Nothing to send, so wait for the client to shut down the stream.
			err := <-errc // a real error, or at least EOF
			if err == io.EOF {
				return nil
			}
			return err
		}
		pr := s.pullResponses[0]
		s.pullResponses = s.pullResponses[1:]
		s.mu.Unlock()
		if pr.err != nil {
			// Add a slight delay to ensure the server receives any
			// messages en route from the client before shutting down the stream.
			// This reduces flakiness of tests involving retry.
			time.Sleep(200 * time.Millisecond)
		}
		if pr.err == io.EOF {
			return nil
		}
		if pr.err != nil {
			return pr.err
		}
		// Return any error from Recv.
		select {
		case err := <-errc:
			return err
		default:
		}
		res := &pb.StreamingPullResponse{ReceivedMessages: pr.msgs}
		if err := stream.Send(res); err != nil {
			return err
		}
	}
}

func (s *mockServer) Acknowledge(ctx context.Context, req *pb.AcknowledgeRequest) (*emptypb.Empty, error) {
	var err error
	s.mu.Lock()
	if len(s.ackErrs) > 0 {
		err = s.ackErrs[0]
		s.ackErrs = s.ackErrs[1:]
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	for _, id := range req.AckIds {
		s.Acked[id] = true
	}
	return &emptypb.Empty{}, nil
}

func (s *mockServer) ModifyAckDeadline(ctx context.Context, req *pb.ModifyAckDeadlineRequest) (*emptypb.Empty, error) {
	var err error
	s.mu.Lock()
	if len(s.modAckErrs) > 0 {
		err = s.modAckErrs[0]
		s.modAckErrs = s.modAckErrs[1:]
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	for _, id := range req.AckIds {
		s.Deadlines[id] = req.AckDeadlineSeconds
	}
	return &emptypb.Empty{}, nil
}

func (s *mockServer) GetSubscription(ctx context.Context, req *pb.GetSubscriptionRequest) (*pb.Subscription, error) {
	return s.sub, nil
}
