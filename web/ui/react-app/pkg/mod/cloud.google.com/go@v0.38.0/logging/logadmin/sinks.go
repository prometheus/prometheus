// Copyright 2016 Google LLC
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

package logadmin

import (
	"context"
	"errors"
	"fmt"

	vkit "cloud.google.com/go/logging/apiv2"
	"google.golang.org/api/iterator"
	logpb "google.golang.org/genproto/googleapis/logging/v2"
	maskpb "google.golang.org/genproto/protobuf/field_mask"
)

// Sink describes a sink used to export log entries outside Stackdriver
// Logging. Incoming log entries matching a filter are exported to a
// destination (a Cloud Storage bucket, BigQuery dataset or Cloud Pub/Sub
// topic).
//
// For more information, see https://cloud.google.com/logging/docs/export/using_exported_logs.
// (The Sinks in this package are what the documentation refers to as "project sinks".)
type Sink struct {
	// ID is a client-assigned sink identifier. Example:
	// "my-severe-errors-to-pubsub".
	// Sink identifiers are limited to 1000 characters
	// and can include only the following characters: A-Z, a-z,
	// 0-9, and the special characters "_-.".
	ID string

	// Destination is the export destination. See
	// https://cloud.google.com/logging/docs/api/tasks/exporting-logs.
	// Examples: "storage.googleapis.com/a-bucket",
	// "bigquery.googleapis.com/projects/a-project-id/datasets/a-dataset".
	Destination string

	// Filter optionally specifies an advanced logs filter (see
	// https://cloud.google.com/logging/docs/view/advanced_filters) that
	// defines the log entries to be exported. Example: "logName:syslog AND
	// severity>=ERROR". If omitted, all entries are returned.
	Filter string

	// WriterIdentity must be a service account name. When exporting logs, Logging
	// adopts this identity for authorization. The export destination's owner must
	// give this service account permission to write to the export destination.
	WriterIdentity string

	// IncludeChildren, when set to true, allows the sink to export log entries from
	// the organization or folder, plus (recursively) from any contained folders, billing
	// accounts, or projects. IncludeChildren is false by default. You can use the sink's
	// filter to choose log entries from specific projects, specific resource types, or
	// specific named logs.
	//
	// Caution: If you enable this feature, your aggregated export sink might export
	// a very large number of log entries. To avoid exporting too many log entries,
	// design your aggregated export sink filter carefully, as described on
	// https://cloud.google.com/logging/docs/export/aggregated_exports.
	IncludeChildren bool
}

// CreateSink creates a Sink. It returns an error if the Sink already exists.
// Requires AdminScope.
func (c *Client) CreateSink(ctx context.Context, sink *Sink) (*Sink, error) {
	return c.CreateSinkOpt(ctx, sink, SinkOptions{})
}

// CreateSinkOpt creates a Sink using the provided options. It returns an
// error if the Sink already exists. Requires AdminScope.
func (c *Client) CreateSinkOpt(ctx context.Context, sink *Sink, opts SinkOptions) (*Sink, error) {
	ls, err := c.sClient.CreateSink(ctx, &logpb.CreateSinkRequest{
		Parent:               c.parent,
		Sink:                 toLogSink(sink),
		UniqueWriterIdentity: opts.UniqueWriterIdentity,
	})
	if err != nil {
		return nil, err
	}
	return fromLogSink(ls), nil
}

// SinkOptions define options to be used when creating or updating a sink.
type SinkOptions struct {
	// Determines the kind of IAM identity returned as WriterIdentity in the new
	// sink. If this value is omitted or set to false, and if the sink's parent is a
	// project, then the value returned as WriterIdentity is the same group or
	// service account used by Stackdriver Logging before the addition of writer
	// identities to the API. The sink's destination must be in the same project as
	// the sink itself.
	//
	// If this field is set to true, or if the sink is owned by a non-project
	// resource such as an organization, then the value of WriterIdentity will
	// be a unique service account used only for exports from the new sink.
	UniqueWriterIdentity bool

	// These fields apply only to UpdateSinkOpt calls. The corresponding sink field
	// is updated if and only if the Update field is true.
	UpdateDestination     bool
	UpdateFilter          bool
	UpdateIncludeChildren bool
}

// DeleteSink deletes a sink. The provided sinkID is the sink's identifier, such as
// "my-severe-errors-to-pubsub".
// Requires AdminScope.
func (c *Client) DeleteSink(ctx context.Context, sinkID string) error {
	return c.sClient.DeleteSink(ctx, &logpb.DeleteSinkRequest{
		SinkName: c.sinkPath(sinkID),
	})
}

// Sink gets a sink. The provided sinkID is the sink's identifier, such as
// "my-severe-errors-to-pubsub".
// Requires ReadScope or AdminScope.
func (c *Client) Sink(ctx context.Context, sinkID string) (*Sink, error) {
	ls, err := c.sClient.GetSink(ctx, &logpb.GetSinkRequest{
		SinkName: c.sinkPath(sinkID),
	})
	if err != nil {
		return nil, err
	}
	return fromLogSink(ls), nil
}

// UpdateSink updates an existing Sink. Requires AdminScope.
//
// WARNING: UpdateSink will always update the Destination, Filter and IncludeChildren
// fields of the sink, even if they have their zero values. Use UpdateSinkOpt
// for more control over which fields to update.
func (c *Client) UpdateSink(ctx context.Context, sink *Sink) (*Sink, error) {
	return c.UpdateSinkOpt(ctx, sink, SinkOptions{
		UpdateDestination:     true,
		UpdateFilter:          true,
		UpdateIncludeChildren: true,
	})
}

// UpdateSinkOpt updates an existing Sink, using the provided options. Requires AdminScope.
//
// To change a sink's writer identity to a service account unique to the sink, set
// opts.UniqueWriterIdentity to true. It is not possible to change a sink's writer identity
// from a unique service account to a non-unique writer identity.
func (c *Client) UpdateSinkOpt(ctx context.Context, sink *Sink, opts SinkOptions) (*Sink, error) {
	mask := &maskpb.FieldMask{}
	if opts.UpdateDestination {
		mask.Paths = append(mask.Paths, "destination")
	}
	if opts.UpdateFilter {
		mask.Paths = append(mask.Paths, "filter")
	}
	if opts.UpdateIncludeChildren {
		mask.Paths = append(mask.Paths, "include_children")
	}
	if opts.UniqueWriterIdentity && len(mask.Paths) == 0 {
		// Hack: specify a deprecated, unchangeable field so that we have a non-empty
		// field mask. (An empty field mask would cause the destination, filter and include_children
		// fields to be changed.)
		mask.Paths = append(mask.Paths, "output_version_format")
	}
	if len(mask.Paths) == 0 {
		return nil, errors.New("logadmin: UpdateSinkOpt: nothing to update")
	}
	ls, err := c.sClient.UpdateSink(ctx, &logpb.UpdateSinkRequest{
		SinkName:             c.sinkPath(sink.ID),
		Sink:                 toLogSink(sink),
		UniqueWriterIdentity: opts.UniqueWriterIdentity,
		UpdateMask:           mask,
	})
	if err != nil {
		return nil, err
	}
	return fromLogSink(ls), err
}

func (c *Client) sinkPath(sinkID string) string {
	return fmt.Sprintf("%s/sinks/%s", c.parent, sinkID)
}

// Sinks returns a SinkIterator for iterating over all Sinks in the Client's project.
// Requires ReadScope or AdminScope.
func (c *Client) Sinks(ctx context.Context) *SinkIterator {
	it := &SinkIterator{
		it: c.sClient.ListSinks(ctx, &logpb.ListSinksRequest{Parent: c.parent}),
	}
	it.pageInfo, it.nextFunc = iterator.NewPageInfo(
		it.fetch,
		func() int { return len(it.items) },
		func() interface{} { b := it.items; it.items = nil; return b })
	return it
}

// A SinkIterator iterates over Sinks.
type SinkIterator struct {
	it       *vkit.LogSinkIterator
	pageInfo *iterator.PageInfo
	nextFunc func() error
	items    []*Sink
}

// PageInfo supports pagination. See the google.golang.org/api/iterator package for details.
func (it *SinkIterator) PageInfo() *iterator.PageInfo { return it.pageInfo }

// Next returns the next result. Its second return value is Done if there are
// no more results. Once Next returns Done, all subsequent calls will return
// Done.
func (it *SinkIterator) Next() (*Sink, error) {
	if err := it.nextFunc(); err != nil {
		return nil, err
	}
	item := it.items[0]
	it.items = it.items[1:]
	return item, nil
}

func (it *SinkIterator) fetch(pageSize int, pageToken string) (string, error) {
	return iterFetch(pageSize, pageToken, it.it.PageInfo(), func() error {
		item, err := it.it.Next()
		if err != nil {
			return err
		}
		it.items = append(it.items, fromLogSink(item))
		return nil
	})
}

func toLogSink(s *Sink) *logpb.LogSink {
	return &logpb.LogSink{
		Name:                s.ID,
		Destination:         s.Destination,
		Filter:              s.Filter,
		IncludeChildren:     s.IncludeChildren,
		OutputVersionFormat: logpb.LogSink_V2,
		// omit WriterIdentity because it is output-only.
	}
}

func fromLogSink(ls *logpb.LogSink) *Sink {
	return &Sink{
		ID:              ls.Name,
		Destination:     ls.Destination,
		Filter:          ls.Filter,
		WriterIdentity:  ls.WriterIdentity,
		IncludeChildren: ls.IncludeChildren,
	}
}
