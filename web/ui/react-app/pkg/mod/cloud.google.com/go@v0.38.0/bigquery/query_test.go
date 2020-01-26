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
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	bq "google.golang.org/api/bigquery/v2"
)

func defaultQueryJob() *bq.Job {
	pfalse := false
	return &bq.Job{
		JobReference: &bq.JobReference{JobId: "RANDOM", ProjectId: "client-project-id"},
		Configuration: &bq.JobConfiguration{
			Query: &bq.JobConfigurationQuery{
				DestinationTable: &bq.TableReference{
					ProjectId: "client-project-id",
					DatasetId: "dataset-id",
					TableId:   "table-id",
				},
				Query: "query string",
				DefaultDataset: &bq.DatasetReference{
					ProjectId: "def-project-id",
					DatasetId: "def-dataset-id",
				},
				UseLegacySql: &pfalse,
			},
		},
	}
}

var defaultQuery = &QueryConfig{
	Q:                "query string",
	DefaultProjectID: "def-project-id",
	DefaultDatasetID: "def-dataset-id",
}

func TestQuery(t *testing.T) {
	defer fixRandomID("RANDOM")()
	c := &Client{
		projectID: "client-project-id",
	}
	testCases := []struct {
		dst         *Table
		src         *QueryConfig
		jobIDConfig JobIDConfig
		want        *bq.Job
	}{
		{
			dst:  c.Dataset("dataset-id").Table("table-id"),
			src:  defaultQuery,
			want: defaultQueryJob(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: &QueryConfig{
				Q:      "query string",
				Labels: map[string]string{"a": "b"},
				DryRun: true,
			},
			want: func() *bq.Job {
				j := defaultQueryJob()
				j.Configuration.Labels = map[string]string{"a": "b"}
				j.Configuration.DryRun = true
				j.Configuration.Query.DefaultDataset = nil
				return j
			}(),
		},
		{
			dst:         c.Dataset("dataset-id").Table("table-id"),
			jobIDConfig: JobIDConfig{JobID: "jobID", AddJobIDSuffix: true},
			src:         &QueryConfig{Q: "query string"},
			want: func() *bq.Job {
				j := defaultQueryJob()
				j.Configuration.Query.DefaultDataset = nil
				j.JobReference.JobId = "jobID-RANDOM"
				return j
			}(),
		},
		{
			dst: &Table{},
			src: defaultQuery,
			want: func() *bq.Job {
				j := defaultQueryJob()
				j.Configuration.Query.DestinationTable = nil
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: &QueryConfig{
				Q: "query string",
				TableDefinitions: map[string]ExternalData{
					"atable": func() *GCSReference {
						g := NewGCSReference("uri")
						g.AllowJaggedRows = true
						g.AllowQuotedNewlines = true
						g.Compression = Gzip
						g.Encoding = UTF_8
						g.FieldDelimiter = ";"
						g.IgnoreUnknownValues = true
						g.MaxBadRecords = 1
						g.Quote = "'"
						g.SkipLeadingRows = 2
						g.Schema = Schema{{Name: "name", Type: StringFieldType}}
						return g
					}(),
				},
			},
			want: func() *bq.Job {
				j := defaultQueryJob()
				j.Configuration.Query.DefaultDataset = nil
				td := make(map[string]bq.ExternalDataConfiguration)
				quote := "'"
				td["atable"] = bq.ExternalDataConfiguration{
					Compression:         "GZIP",
					IgnoreUnknownValues: true,
					MaxBadRecords:       1,
					SourceFormat:        "CSV", // must be explicitly set.
					SourceUris:          []string{"uri"},
					CsvOptions: &bq.CsvOptions{
						AllowJaggedRows:     true,
						AllowQuotedNewlines: true,
						Encoding:            "UTF-8",
						FieldDelimiter:      ";",
						SkipLeadingRows:     2,
						Quote:               &quote,
					},
					Schema: &bq.TableSchema{
						Fields: []*bq.TableFieldSchema{
							{Name: "name", Type: "STRING"},
						},
					},
				}
				j.Configuration.Query.TableDefinitions = td
				return j
			}(),
		},
		{
			dst: &Table{
				ProjectID: "project-id",
				DatasetID: "dataset-id",
				TableID:   "table-id",
			},
			src: &QueryConfig{
				Q:                 "query string",
				DefaultProjectID:  "def-project-id",
				DefaultDatasetID:  "def-dataset-id",
				CreateDisposition: CreateNever,
				WriteDisposition:  WriteTruncate,
			},
			want: func() *bq.Job {
				j := defaultQueryJob()
				j.Configuration.Query.DestinationTable.ProjectId = "project-id"
				j.Configuration.Query.WriteDisposition = "WRITE_TRUNCATE"
				j.Configuration.Query.CreateDisposition = "CREATE_NEVER"
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: &QueryConfig{
				Q:                 "query string",
				DefaultProjectID:  "def-project-id",
				DefaultDatasetID:  "def-dataset-id",
				DisableQueryCache: true,
			},
			want: func() *bq.Job {
				j := defaultQueryJob()
				f := false
				j.Configuration.Query.UseQueryCache = &f
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: &QueryConfig{
				Q:                 "query string",
				DefaultProjectID:  "def-project-id",
				DefaultDatasetID:  "def-dataset-id",
				AllowLargeResults: true,
			},
			want: func() *bq.Job {
				j := defaultQueryJob()
				j.Configuration.Query.AllowLargeResults = true
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: &QueryConfig{
				Q:                       "query string",
				DefaultProjectID:        "def-project-id",
				DefaultDatasetID:        "def-dataset-id",
				DisableFlattenedResults: true,
			},
			want: func() *bq.Job {
				j := defaultQueryJob()
				f := false
				j.Configuration.Query.FlattenResults = &f
				j.Configuration.Query.AllowLargeResults = true
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: &QueryConfig{
				Q:                "query string",
				DefaultProjectID: "def-project-id",
				DefaultDatasetID: "def-dataset-id",
				Priority:         QueryPriority("low"),
			},
			want: func() *bq.Job {
				j := defaultQueryJob()
				j.Configuration.Query.Priority = "low"
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: &QueryConfig{
				Q:                "query string",
				DefaultProjectID: "def-project-id",
				DefaultDatasetID: "def-dataset-id",
				MaxBillingTier:   3,
				MaxBytesBilled:   5,
			},
			want: func() *bq.Job {
				j := defaultQueryJob()
				tier := int64(3)
				j.Configuration.Query.MaximumBillingTier = &tier
				j.Configuration.Query.MaximumBytesBilled = 5
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: &QueryConfig{
				Q:                "query string",
				DefaultProjectID: "def-project-id",
				DefaultDatasetID: "def-dataset-id",
				UseStandardSQL:   true,
			},
			want: defaultQueryJob(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: &QueryConfig{
				Q:                "query string",
				DefaultProjectID: "def-project-id",
				DefaultDatasetID: "def-dataset-id",
				UseLegacySQL:     true,
			},
			want: func() *bq.Job {
				j := defaultQueryJob()
				ptrue := true
				j.Configuration.Query.UseLegacySql = &ptrue
				j.Configuration.Query.ForceSendFields = nil
				return j
			}(),
		},
	}
	for i, tc := range testCases {
		query := c.Query("")
		query.JobIDConfig = tc.jobIDConfig
		query.QueryConfig = *tc.src
		query.Dst = tc.dst
		got, err := query.newJob()
		if err != nil {
			t.Errorf("#%d: err calling query: %v", i, err)
			continue
		}
		checkJob(t, i, got, tc.want)

		// Round-trip.
		jc, err := bqToJobConfig(got.Configuration, c)
		if err != nil {
			t.Fatalf("#%d: %v", i, err)
		}
		wantConfig := query.QueryConfig
		// We set AllowLargeResults to true when DisableFlattenedResults is true.
		if wantConfig.DisableFlattenedResults {
			wantConfig.AllowLargeResults = true
		}
		// A QueryConfig with neither UseXXXSQL field set is equivalent
		// to one where UseStandardSQL = true.
		if !wantConfig.UseLegacySQL && !wantConfig.UseStandardSQL {
			wantConfig.UseStandardSQL = true
		}
		// Treat nil and empty tables the same, and ignore the client.
		tableEqual := func(t1, t2 *Table) bool {
			if t1 == nil {
				t1 = &Table{}
			}
			if t2 == nil {
				t2 = &Table{}
			}
			return t1.ProjectID == t2.ProjectID && t1.DatasetID == t2.DatasetID && t1.TableID == t2.TableID
		}
		// A table definition that is a GCSReference round-trips as an ExternalDataConfig.
		// TODO(jba): see if there is a way to express this with a transformer.
		gcsRefToEDC := func(g *GCSReference) *ExternalDataConfig {
			q := g.toBQ()
			e, _ := bqToExternalDataConfig(&q)
			return e
		}
		externalDataEqual := func(e1, e2 ExternalData) bool {
			if r, ok := e1.(*GCSReference); ok {
				e1 = gcsRefToEDC(r)
			}
			if r, ok := e2.(*GCSReference); ok {
				e2 = gcsRefToEDC(r)
			}
			return cmp.Equal(e1, e2)
		}
		diff := testutil.Diff(jc.(*QueryConfig), &wantConfig,
			cmp.Comparer(tableEqual),
			cmp.Comparer(externalDataEqual),
		)
		if diff != "" {
			t.Errorf("#%d: (got=-, want=+:\n%s", i, diff)
		}
	}
}

func TestConfiguringQuery(t *testing.T) {
	c := &Client{
		projectID: "project-id",
	}

	query := c.Query("q")
	query.JobID = "ajob"
	query.DefaultProjectID = "def-project-id"
	query.DefaultDatasetID = "def-dataset-id"
	query.TimePartitioning = &TimePartitioning{Expiration: 1234 * time.Second, Field: "f"}
	query.Clustering = &Clustering{
		Fields: []string{"cfield1"},
	}
	query.DestinationEncryptionConfig = &EncryptionConfig{KMSKeyName: "keyName"}
	query.SchemaUpdateOptions = []string{"ALLOW_FIELD_ADDITION"}

	// Note: Other configuration fields are tested in other tests above.
	// A lot of that can be consolidated once Client.Copy is gone.

	pfalse := false
	want := &bq.Job{
		Configuration: &bq.JobConfiguration{
			Query: &bq.JobConfigurationQuery{
				Query: "q",
				DefaultDataset: &bq.DatasetReference{
					ProjectId: "def-project-id",
					DatasetId: "def-dataset-id",
				},
				UseLegacySql:                       &pfalse,
				TimePartitioning:                   &bq.TimePartitioning{ExpirationMs: 1234000, Field: "f", Type: "DAY"},
				Clustering:                         &bq.Clustering{Fields: []string{"cfield1"}},
				DestinationEncryptionConfiguration: &bq.EncryptionConfiguration{KmsKeyName: "keyName"},
				SchemaUpdateOptions:                []string{"ALLOW_FIELD_ADDITION"},
			},
		},
		JobReference: &bq.JobReference{
			JobId:     "ajob",
			ProjectId: "project-id",
		},
	}

	got, err := query.newJob()
	if err != nil {
		t.Fatalf("err calling Query.newJob: %v", err)
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("querying: -got +want:\n%s", diff)
	}
}

func TestQueryLegacySQL(t *testing.T) {
	c := &Client{projectID: "project-id"}
	q := c.Query("q")
	q.UseStandardSQL = true
	q.UseLegacySQL = true
	_, err := q.newJob()
	if err == nil {
		t.Error("UseStandardSQL and UseLegacySQL: got nil, want error")
	}
	q = c.Query("q")
	q.Parameters = []QueryParameter{{Name: "p", Value: 3}}
	q.UseLegacySQL = true
	_, err = q.newJob()
	if err == nil {
		t.Error("Parameters and UseLegacySQL: got nil, want error")
	}
}
