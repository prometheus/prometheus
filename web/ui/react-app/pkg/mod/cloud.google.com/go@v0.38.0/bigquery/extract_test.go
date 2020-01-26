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
	"github.com/google/go-cmp/cmp"
	bq "google.golang.org/api/bigquery/v2"
)

func defaultExtractJob() *bq.Job {
	return &bq.Job{
		JobReference: &bq.JobReference{JobId: "RANDOM", ProjectId: "client-project-id"},
		Configuration: &bq.JobConfiguration{
			Extract: &bq.JobConfigurationExtract{
				SourceTable: &bq.TableReference{
					ProjectId: "client-project-id",
					DatasetId: "dataset-id",
					TableId:   "table-id",
				},
				DestinationUris: []string{"uri"},
			},
		},
	}
}

func defaultGCS() *GCSReference {
	return &GCSReference{
		URIs: []string{"uri"},
	}
}

func TestExtract(t *testing.T) {
	defer fixRandomID("RANDOM")()
	c := &Client{
		projectID: "client-project-id",
	}

	testCases := []struct {
		dst    *GCSReference
		src    *Table
		config ExtractConfig
		want   *bq.Job
	}{
		{
			dst:  defaultGCS(),
			src:  c.Dataset("dataset-id").Table("table-id"),
			want: defaultExtractJob(),
		},
		{
			dst: defaultGCS(),
			src: c.Dataset("dataset-id").Table("table-id"),
			config: ExtractConfig{
				DisableHeader: true,
				Labels:        map[string]string{"a": "b"},
			},
			want: func() *bq.Job {
				j := defaultExtractJob()
				j.Configuration.Labels = map[string]string{"a": "b"}
				f := false
				j.Configuration.Extract.PrintHeader = &f
				return j
			}(),
		},
		{
			dst: func() *GCSReference {
				g := NewGCSReference("uri")
				g.Compression = Gzip
				g.DestinationFormat = JSON
				g.FieldDelimiter = "\t"
				return g
			}(),
			src: c.Dataset("dataset-id").Table("table-id"),
			want: func() *bq.Job {
				j := defaultExtractJob()
				j.Configuration.Extract.Compression = "GZIP"
				j.Configuration.Extract.DestinationFormat = "NEWLINE_DELIMITED_JSON"
				j.Configuration.Extract.FieldDelimiter = "\t"
				return j
			}(),
		},
	}

	for i, tc := range testCases {
		ext := tc.src.ExtractorTo(tc.dst)
		tc.config.Src = ext.Src
		tc.config.Dst = ext.Dst
		ext.ExtractConfig = tc.config
		got := ext.newJob()
		checkJob(t, i, got, tc.want)

		jc, err := bqToJobConfig(got.Configuration, c)
		if err != nil {
			t.Fatalf("#%d: %v", i, err)
		}
		diff := testutil.Diff(jc, &ext.ExtractConfig,
			cmp.AllowUnexported(Table{}, Client{}))
		if diff != "" {
			t.Errorf("#%d: (got=-, want=+:\n%s", i, diff)
		}
	}
}
