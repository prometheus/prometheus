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

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"
	pb "google.golang.org/genproto/googleapis/pubsub/v1"
)

// Snapshot is a reference to a PubSub snapshot.
type Snapshot struct {
	c *Client

	// The fully qualified identifier for the snapshot, in the format "projects/<projid>/snapshots/<snap>"
	name string
}

// ID returns the unique identifier of the snapshot within its project.
func (s *Snapshot) ID() string {
	slash := strings.LastIndex(s.name, "/")
	if slash == -1 {
		// name is not a fully-qualified name.
		panic("bad snapshot name")
	}
	return s.name[slash+1:]
}

// SnapshotConfig contains the details of a Snapshot.
type SnapshotConfig struct {
	*Snapshot
	Topic      *Topic
	Expiration time.Time
}

// Snapshot creates a reference to a snapshot.
func (c *Client) Snapshot(id string) *Snapshot {
	return &Snapshot{
		c:    c,
		name: fmt.Sprintf("projects/%s/snapshots/%s", c.projectID, id),
	}
}

// Snapshots returns an iterator which returns snapshots for this project.
func (c *Client) Snapshots(ctx context.Context) *SnapshotConfigIterator {
	it := c.subc.ListSnapshots(ctx, &pb.ListSnapshotsRequest{
		Project: c.fullyQualifiedProjectName(),
	})
	next := func() (*SnapshotConfig, error) {
		snap, err := it.Next()
		if err != nil {
			return nil, err
		}
		return toSnapshotConfig(snap, c)
	}
	return &SnapshotConfigIterator{next: next}
}

// SnapshotConfigIterator is an iterator that returns a series of snapshots.
type SnapshotConfigIterator struct {
	next func() (*SnapshotConfig, error)
}

// Next returns the next SnapshotConfig. Its second return value is iterator.Done if there are no more results.
// Once Next returns iterator.Done, all subsequent calls will return iterator.Done.
func (snaps *SnapshotConfigIterator) Next() (*SnapshotConfig, error) {
	return snaps.next()
}

// Delete deletes a snapshot.
func (s *Snapshot) Delete(ctx context.Context) error {
	return s.c.subc.DeleteSnapshot(ctx, &pb.DeleteSnapshotRequest{Snapshot: s.name})
}

// SeekToTime seeks the subscription to a point in time.
//
// Messages retained in the subscription that were published before this
// time are marked as acknowledged, and messages retained in the
// subscription that were published after this time are marked as
// unacknowledged. Note that this operation affects only those messages
// retained in the subscription (configured by SnapshotConfig). For example,
// if `time` corresponds to a point before the message retention
// window (or to a point before the system's notion of the subscription
// creation time), only retained messages will be marked as unacknowledged,
// and already-expunged messages will not be restored.
func (s *Subscription) SeekToTime(ctx context.Context, t time.Time) error {
	ts, err := ptypes.TimestampProto(t)
	if err != nil {
		return err
	}
	_, err = s.c.subc.Seek(ctx, &pb.SeekRequest{
		Subscription: s.name,
		Target:       &pb.SeekRequest_Time{ts},
	})
	return err
}

// CreateSnapshot creates a new snapshot from this subscription.
// The snapshot will be for the topic this subscription is subscribed to.
// If the name is empty string, a unique name is assigned.
//
// The created snapshot is guaranteed to retain:
//  (a) The existing backlog on the subscription. More precisely, this is
//      defined as the messages in the subscription's backlog that are
//      unacknowledged when Snapshot returns without error.
//  (b) Any messages published to the subscription's topic following
//      Snapshot returning without error.
func (s *Subscription) CreateSnapshot(ctx context.Context, name string) (*SnapshotConfig, error) {
	if name != "" {
		name = fmt.Sprintf("projects/%s/snapshots/%s", strings.Split(s.name, "/")[1], name)
	}
	snap, err := s.c.subc.CreateSnapshot(ctx, &pb.CreateSnapshotRequest{
		Name:         name,
		Subscription: s.name,
	})
	if err != nil {
		return nil, err
	}
	return toSnapshotConfig(snap, s.c)
}

// SeekToSnapshot seeks the subscription to a snapshot.
//
// The snapshot need not be created from this subscription,
// but it must be for the topic this subscription is subscribed to.
func (s *Subscription) SeekToSnapshot(ctx context.Context, snap *Snapshot) error {
	_, err := s.c.subc.Seek(ctx, &pb.SeekRequest{
		Subscription: s.name,
		Target:       &pb.SeekRequest_Snapshot{snap.name},
	})
	return err
}

func toSnapshotConfig(snap *pb.Snapshot, c *Client) (*SnapshotConfig, error) {
	exp, err := ptypes.Timestamp(snap.ExpireTime)
	if err != nil {
		return nil, err
	}
	return &SnapshotConfig{
		Snapshot:   &Snapshot{c: c, name: snap.Name},
		Topic:      newTopic(c, snap.Topic),
		Expiration: exp,
	}, nil
}
