// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// AUTO-GENERATED CODE. DO NOT EDIT.

package logging

import (
	loggingpb "google.golang.org/genproto/googleapis/logging/v2"
)

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

var _ = fmt.Sprintf
var _ = iterator.Done
var _ = strconv.FormatUint
var _ = time.Now

func TestLoggingServiceV2Smoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping smoke test in short mode")
	}
	ctx := context.Background()
	ts := testutil.TokenSource(ctx, DefaultAuthScopes()...)
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}

	projectId := testutil.ProjID()
	_ = projectId

	c, err := NewClient(ctx, option.WithTokenSource(ts))
	if err != nil {
		t.Fatal(err)
	}

	var entries []*loggingpb.LogEntry = nil
	var formattedLogName string = fmt.Sprintf("projects/%s/logs/%s", projectId, "test-"+strconv.FormatInt(time.Now().UnixNano(), 10)+"")
	var request = &loggingpb.WriteLogEntriesRequest{
		Entries: entries,
		LogName: formattedLogName,
	}

	if _, err := c.WriteLogEntries(ctx, request); err != nil {
		t.Error(err)
	}
}
