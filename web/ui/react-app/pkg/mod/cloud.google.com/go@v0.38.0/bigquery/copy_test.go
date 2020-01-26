// Copyright 2015 Google LLC
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

package bigquery

import (
	"testing"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp/cmpopts"
	bq "google.golang.org/api/bigquery/v2"
)

func defaultCopyJob() *bq.Job {
	return &bq.Job{
		JobReference: &bq.JobReference{JobId: "RANDOM", ProjectId: "client-project-id"},
		Configuration: &bq.JobConfiguration{
			Copy: &bq.JobConfigurationTableCopy{
				DestinationTable: &bq.TableReference{
					ProjectId: "d-project-id",
					DatasetId: "d-dataset-id",
					TableId:   "d-table-id",
				},
				SourceTables: []*bq.TableReference{
					{
						ProjectId: "s-project-id",
						DatasetId: "s-dataset-id",
						TableId:   "s-table-id",
					},
				},
			},
		},
	}
}

func TestCopy(t *testing.T) {
	defer fixRandomID("RANDOM")()
	testCases := []struct {
		dst      *Table
		srcs     []*Table
		jobID    string
		location string
		config   CopyConfig
		want     *bq.Job
	}{
		{
			dst: &Table{
				ProjectID: "d-project-id",
				DatasetID: "d-dataset-id",
				TableID:   "d-table-id",
			},
			srcs: []*Table{
				{
					ProjectID: "s-project-id",
					DatasetID: "s-dataset-id",
					TableID:   "s-table-id",
				},
			},
			want: defaultCopyJob(),
		},
		{
			dst: &Table{
				ProjectID: "d-project-id",
				DatasetID: "d-dataset-id",
				TableID:   "d-table-id",
			},
			srcs: []*Table{
				{
					ProjectID: "s-project-id",
					DatasetID: "s-dataset-id",
					TableID:   "s-table-id",
				},
			},
			config: CopyConfig{
				CreateDisposition:           CreateNever,
				WriteDisposition:            WriteTruncate,
				DestinationEncryptionConfig: &EncryptionConfig{KMSKeyName: "keyName"},
				Labels:                      map[string]string{"a": "b"},
			},
			want: func() *bq.Job {
				j := defaultCopyJob()
				j.Configuration.Labels = map[string]string{"a": "b"}
				j.Configuration.Copy.CreateDisposition = "CREATE_NEVER"
				j.Configuration.Copy.WriteDisposition = "WRITE_TRUNCATE"
				j.Configuration.Copy.DestinationEncryptionConfiguration = &bq.EncryptionConfiguration{KmsKeyName: "keyName"}
				return j
			}(),
		},
		{
			dst: &Table{
				ProjectID: "d-project-id",
				DatasetID: "d-dataset-id",
				TableID:   "d-table-id",
			},
			srcs: []*Table{
				{
					ProjectID: "s-project-id",
					DatasetID: "s-dataset-id",
					TableID:   "s-table-id",
				},
			},
			jobID: "job-id",
			want: func() *bq.Job {
				j := defaultCopyJob()
				j.JobReference.JobId = "job-id"
				return j
			}(),
		},
		{
			dst: &Table{
				ProjectID: "d-project-id",
				DatasetID: "d-dataset-id",
				TableID:   "d-table-id",
			},
			srcs: []*Table{
				{
					ProjectID: "s-project-id",
					DatasetID: "s-dataset-id",
					TableID:   "s-table-id",
				},
			},
			location: "asia-northeast1",
			want: func() *bq.Job {
				j := defaultCopyJob()
				j.JobReference.Location = "asia-northeast1"
				return j
			}(),
		},
	}
	c := &Client{projectID: "client-project-id"}
	for i, tc := range testCases {
		tc.dst.c = c
		copier := tc.dst.CopierFrom(tc.srcs...)
		copier.JobID = tc.jobID
		copier.Location = tc.location
		tc.config.Srcs = tc.srcs
		tc.config.Dst = tc.dst
		copier.CopyConfig = tc.config
		got := copier.newJob()
		checkJob(t, i, got, tc.want)

		jc, err := bqToJobConfig(got.Configuration, c)
		if err != nil {
			t.Fatalf("#%d: %v", i, err)
		}
		diff := testutil.Diff(jc.(*CopyConfig), &copier.CopyConfig,
			cmpopts.IgnoreUnexported(Table{}))
		if diff != "" {
			t.Errorf("#%d: (got=-, want=+:\n%s", i, diff)
		}
	}
}
