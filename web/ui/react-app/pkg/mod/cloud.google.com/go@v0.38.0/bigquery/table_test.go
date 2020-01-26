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

package bigquery

import (
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	bq "google.golang.org/api/bigquery/v2"
)

func TestBQToTableMetadata(t *testing.T) {
	aTime := time.Date(2017, 1, 26, 0, 0, 0, 0, time.Local)
	aTimeMillis := aTime.UnixNano() / 1e6
	for _, test := range []struct {
		in   *bq.Table
		want *TableMetadata
	}{
		{&bq.Table{}, &TableMetadata{}}, // test minimal case
		{
			&bq.Table{
				CreationTime:     aTimeMillis,
				Description:      "desc",
				Etag:             "etag",
				ExpirationTime:   aTimeMillis,
				FriendlyName:     "fname",
				Id:               "id",
				LastModifiedTime: uint64(aTimeMillis),
				Location:         "loc",
				NumBytes:         123,
				NumLongTermBytes: 23,
				NumRows:          7,
				StreamingBuffer: &bq.Streamingbuffer{
					EstimatedBytes:  11,
					EstimatedRows:   3,
					OldestEntryTime: uint64(aTimeMillis),
				},
				TimePartitioning: &bq.TimePartitioning{
					ExpirationMs: 7890,
					Type:         "DAY",
					Field:        "pfield",
				},
				Clustering: &bq.Clustering{
					Fields: []string{"cfield1", "cfield2"},
				},
				EncryptionConfiguration: &bq.EncryptionConfiguration{KmsKeyName: "keyName"},
				Type:                    "EXTERNAL",
				View:                    &bq.ViewDefinition{Query: "view-query"},
				Labels:                  map[string]string{"a": "b"},
				ExternalDataConfiguration: &bq.ExternalDataConfiguration{
					SourceFormat: "GOOGLE_SHEETS",
				},
			},
			&TableMetadata{
				Description:        "desc",
				Name:               "fname",
				ViewQuery:          "view-query",
				FullID:             "id",
				Type:               ExternalTable,
				Labels:             map[string]string{"a": "b"},
				ExternalDataConfig: &ExternalDataConfig{SourceFormat: GoogleSheets},
				ExpirationTime:     aTime.Truncate(time.Millisecond),
				CreationTime:       aTime.Truncate(time.Millisecond),
				LastModifiedTime:   aTime.Truncate(time.Millisecond),
				NumBytes:           123,
				NumLongTermBytes:   23,
				NumRows:            7,
				TimePartitioning: &TimePartitioning{
					Expiration: 7890 * time.Millisecond,
					Field:      "pfield",
				},
				Clustering: &Clustering{
					Fields: []string{"cfield1", "cfield2"},
				},
				StreamingBuffer: &StreamingBuffer{
					EstimatedBytes:  11,
					EstimatedRows:   3,
					OldestEntryTime: aTime,
				},
				EncryptionConfig: &EncryptionConfig{KMSKeyName: "keyName"},
				ETag:             "etag",
			},
		},
	} {
		got, err := bqToTableMetadata(test.in)
		if err != nil {
			t.Fatal(err)
		}
		if diff := testutil.Diff(got, test.want); diff != "" {
			t.Errorf("%+v:\n, -got, +want:\n%s", test.in, diff)
		}
	}
}

func TestTableMetadataToBQ(t *testing.T) {
	aTime := time.Date(2017, 1, 26, 0, 0, 0, 0, time.Local)
	aTimeMillis := aTime.UnixNano() / 1e6
	sc := Schema{fieldSchema("desc", "name", "STRING", false, true)}

	for _, test := range []struct {
		in   *TableMetadata
		want *bq.Table
	}{
		{nil, &bq.Table{}},
		{&TableMetadata{}, &bq.Table{}},
		{
			&TableMetadata{
				Name:               "n",
				Description:        "d",
				Schema:             sc,
				ExpirationTime:     aTime,
				Labels:             map[string]string{"a": "b"},
				ExternalDataConfig: &ExternalDataConfig{SourceFormat: Bigtable},
				EncryptionConfig:   &EncryptionConfig{KMSKeyName: "keyName"},
			},
			&bq.Table{
				FriendlyName: "n",
				Description:  "d",
				Schema: &bq.TableSchema{
					Fields: []*bq.TableFieldSchema{
						bqTableFieldSchema("desc", "name", "STRING", "REQUIRED"),
					},
				},
				ExpirationTime:            aTimeMillis,
				Labels:                    map[string]string{"a": "b"},
				ExternalDataConfiguration: &bq.ExternalDataConfiguration{SourceFormat: "BIGTABLE"},
				EncryptionConfiguration:   &bq.EncryptionConfiguration{KmsKeyName: "keyName"},
			},
		},
		{
			&TableMetadata{ViewQuery: "q"},
			&bq.Table{
				View: &bq.ViewDefinition{
					Query:           "q",
					UseLegacySql:    false,
					ForceSendFields: []string{"UseLegacySql"},
				},
			},
		},
		{
			&TableMetadata{
				ViewQuery:        "q",
				UseLegacySQL:     true,
				TimePartitioning: &TimePartitioning{},
			},
			&bq.Table{
				View: &bq.ViewDefinition{
					Query:        "q",
					UseLegacySql: true,
				},
				TimePartitioning: &bq.TimePartitioning{
					Type:         "DAY",
					ExpirationMs: 0,
				},
			},
		},
		{
			&TableMetadata{
				ViewQuery:      "q",
				UseStandardSQL: true,
				TimePartitioning: &TimePartitioning{
					Expiration: time.Second,
					Field:      "ofDreams",
				},
				Clustering: &Clustering{
					Fields: []string{"cfield1"},
				},
			},
			&bq.Table{
				View: &bq.ViewDefinition{
					Query:           "q",
					UseLegacySql:    false,
					ForceSendFields: []string{"UseLegacySql"},
				},
				TimePartitioning: &bq.TimePartitioning{
					Type:         "DAY",
					ExpirationMs: 1000,
					Field:        "ofDreams",
				},
				Clustering: &bq.Clustering{
					Fields: []string{"cfield1"},
				},
			},
		},
		{
			&TableMetadata{ExpirationTime: NeverExpire},
			&bq.Table{ExpirationTime: 0},
		},
	} {
		got, err := test.in.toBQ()
		if err != nil {
			t.Fatalf("%+v: %v", test.in, err)
		}
		if diff := testutil.Diff(got, test.want); diff != "" {
			t.Errorf("%+v:\n-got, +want:\n%s", test.in, diff)
		}
	}

	// Errors
	for _, in := range []*TableMetadata{
		{Schema: sc, ViewQuery: "q"}, // can't have both schema and query
		{UseLegacySQL: true},         // UseLegacySQL without query
		{UseStandardSQL: true},       // UseStandardSQL without query
		// read-only fields
		{FullID: "x"},
		{Type: "x"},
		{CreationTime: aTime},
		{LastModifiedTime: aTime},
		{NumBytes: 1},
		{NumLongTermBytes: 1},
		{NumRows: 1},
		{StreamingBuffer: &StreamingBuffer{}},
		{ETag: "x"},
		// expiration time outside allowable range is invalid
		// See https://godoc.org/time#Time.UnixNano
		{ExpirationTime: time.Date(1677, 9, 21, 0, 12, 43, 145224192, time.UTC).Add(-1)},
		{ExpirationTime: time.Date(2262, 04, 11, 23, 47, 16, 854775807, time.UTC).Add(1)},
	} {
		_, err := in.toBQ()
		if err == nil {
			t.Errorf("%+v: got nil, want error", in)
		}
	}
}

func TestTableMetadataToUpdateToBQ(t *testing.T) {
	aTime := time.Date(2017, 1, 26, 0, 0, 0, 0, time.Local)
	for _, test := range []struct {
		tm   TableMetadataToUpdate
		want *bq.Table
	}{
		{
			tm:   TableMetadataToUpdate{},
			want: &bq.Table{},
		},
		{
			tm: TableMetadataToUpdate{
				Description: "d",
				Name:        "n",
			},
			want: &bq.Table{
				Description:     "d",
				FriendlyName:    "n",
				ForceSendFields: []string{"Description", "FriendlyName"},
			},
		},
		{
			tm: TableMetadataToUpdate{
				Schema:         Schema{fieldSchema("desc", "name", "STRING", false, true)},
				ExpirationTime: aTime,
			},
			want: &bq.Table{
				Schema: &bq.TableSchema{
					Fields: []*bq.TableFieldSchema{
						bqTableFieldSchema("desc", "name", "STRING", "REQUIRED"),
					},
				},
				ExpirationTime:  aTime.UnixNano() / 1e6,
				ForceSendFields: []string{"Schema", "ExpirationTime"},
			},
		},
		{
			tm: TableMetadataToUpdate{ViewQuery: "q"},
			want: &bq.Table{
				View: &bq.ViewDefinition{Query: "q", ForceSendFields: []string{"Query"}},
			},
		},
		{
			tm: TableMetadataToUpdate{UseLegacySQL: false},
			want: &bq.Table{
				View: &bq.ViewDefinition{
					UseLegacySql:    false,
					ForceSendFields: []string{"UseLegacySql"},
				},
			},
		},
		{
			tm: TableMetadataToUpdate{ViewQuery: "q", UseLegacySQL: true},
			want: &bq.Table{
				View: &bq.ViewDefinition{
					Query:           "q",
					UseLegacySql:    true,
					ForceSendFields: []string{"Query", "UseLegacySql"},
				},
			},
		},
		{
			tm: func() (tm TableMetadataToUpdate) {
				tm.SetLabel("L", "V")
				tm.DeleteLabel("D")
				return tm
			}(),
			want: &bq.Table{
				Labels:     map[string]string{"L": "V"},
				NullFields: []string{"Labels.D"},
			},
		},
		{
			tm: TableMetadataToUpdate{ExpirationTime: NeverExpire},
			want: &bq.Table{
				NullFields: []string{"ExpirationTime"},
			},
		},
		{
			tm: TableMetadataToUpdate{TimePartitioning: &TimePartitioning{Expiration: 0}},
			want: &bq.Table{
				TimePartitioning: &bq.TimePartitioning{
					Type:            "DAY",
					ForceSendFields: []string{"RequirePartitionFilter"},
					NullFields:      []string{"ExpirationMs"},
				},
			},
		},
		{
			tm: TableMetadataToUpdate{TimePartitioning: &TimePartitioning{Expiration: time.Duration(time.Hour)}},
			want: &bq.Table{
				TimePartitioning: &bq.TimePartitioning{
					ExpirationMs:    3600000,
					Type:            "DAY",
					ForceSendFields: []string{"RequirePartitionFilter"},
				},
			},
		},
	} {
		got, _ := test.tm.toBQ()
		if !testutil.Equal(got, test.want) {
			t.Errorf("%+v:\ngot  %+v\nwant %+v", test.tm, got, test.want)
		}
	}
}

func TestTableMetadataToUpdateToBQErrors(t *testing.T) {
	// See https://godoc.org/time#Time.UnixNano
	start := time.Date(1677, 9, 21, 0, 12, 43, 145224192, time.UTC)
	end := time.Date(2262, 04, 11, 23, 47, 16, 854775807, time.UTC)

	for _, test := range []struct {
		desc    string
		aTime   time.Time
		wantErr bool
	}{
		{desc: "ignored zero value", aTime: time.Time{}, wantErr: false},
		{desc: "earliest valid time", aTime: start, wantErr: false},
		{desc: "latested valid time", aTime: end, wantErr: false},
		{desc: "invalid times before 1678", aTime: start.Add(-1), wantErr: true},
		{desc: "invalid times after 2262", aTime: end.Add(1), wantErr: true},
		{desc: "valid times after 1678", aTime: start.Add(1), wantErr: false},
		{desc: "valid times before 2262", aTime: end.Add(-1), wantErr: false},
	} {
		tm := &TableMetadataToUpdate{ExpirationTime: test.aTime}
		_, err := tm.toBQ()
		if test.wantErr && err == nil {
			t.Errorf("[%s] got no error, want error", test.desc)
		}
		if !test.wantErr && err != nil {
			t.Errorf("[%s] got error, want no error", test.desc)
		}
	}
}
