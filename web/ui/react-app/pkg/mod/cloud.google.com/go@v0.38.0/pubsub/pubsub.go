// Copyright 2014 Google LLC
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

package pubsub // import "cloud.google.com/go/pubsub"

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"cloud.google.com/go/internal/version"
	vkit "cloud.google.com/go/pubsub/apiv1"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

const (
	// ScopePubSub grants permissions to view and manage Pub/Sub
	// topics and subscriptions.
	ScopePubSub = "https://www.googleapis.com/auth/pubsub"

	// ScopeCloudPlatform grants permissions to view and manage your data
	// across Google Cloud Platform services.
	ScopeCloudPlatform = "https://www.googleapis.com/auth/cloud-platform"

	maxAckDeadline = 10 * time.Minute
)

// Client is a Google Pub/Sub client scoped to a single project.
//
// Clients should be reused rather than being created as needed.
// A Client may be shared by multiple goroutines.
type Client struct {
	projectID string
	pubc      *vkit.PublisherClient
	subc      *vkit.SubscriberClient
}

// NewClient creates a new PubSub client.
func NewClient(ctx context.Context, projectID string, opts ...option.ClientOption) (c *Client, err error) {
	var o []option.ClientOption
	// Environment variables for gcloud emulator:
	// https://cloud.google.com/sdk/gcloud/reference/beta/emulators/pubsub/
	if addr := os.Getenv("PUBSUB_EMULATOR_HOST"); addr != "" {
		conn, err := grpc.Dial(addr, grpc.WithInsecure())
		if err != nil {
			return nil, fmt.Errorf("grpc.Dial: %v", err)
		}
		o = []option.ClientOption{option.WithGRPCConn(conn)}
	} else {
		o = []option.ClientOption{
			// Create multiple connections to increase throughput.
			option.WithGRPCConnectionPool(runtime.GOMAXPROCS(0)),
			option.WithGRPCDialOption(grpc.WithKeepaliveParams(keepalive.ClientParameters{
				Time: 5 * time.Minute,
			})),
		}
		o = append(o, openCensusOptions()...)
	}
	o = append(o, opts...)
	pubc, err := vkit.NewPublisherClient(ctx, o...)
	if err != nil {
		return nil, fmt.Errorf("pubsub: %v", err)
	}
	subc, err := vkit.NewSubscriberClient(ctx, option.WithGRPCConn(pubc.Connection()))
	if err != nil {
		// Should never happen, since we are passing in the connection.
		// If it does, we cannot close, because the user may have passed in their
		// own connection originally.
		return nil, fmt.Errorf("pubsub: %v", err)
	}
	pubc.SetGoogleClientInfo("gccl", version.Repo)
	subc.SetGoogleClientInfo("gccl", version.Repo)
	return &Client{
		projectID: projectID,
		pubc:      pubc,
		subc:      subc,
	}, nil
}

// Close releases any resources held by the client,
// such as memory and goroutines.
//
// If the client is available for the lifetime of the program, then Close need not be
// called at exit.
func (c *Client) Close() error {
	// Return the first error, because the first call closes the connection.
	err := c.pubc.Close()
	_ = c.subc.Close()
	return err
}

func (c *Client) fullyQualifiedProjectName() string {
	return fmt.Sprintf("projects/%s", c.projectID)
}
