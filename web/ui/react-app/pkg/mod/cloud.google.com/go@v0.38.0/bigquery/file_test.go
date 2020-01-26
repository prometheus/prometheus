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

package bigquery

import (
	"testing"

	"cloud.google.com/go/internal/pretty"
	"cloud.google.com/go/internal/testutil"
	bq "google.golang.org/api/bigquery/v2"
)

var (
	hyphen = "-"
	fc     = FileConfig{
		SourceFormat:        CSV,
		AutoDetect:          true,
		MaxBadRecords:       7,
		IgnoreUnknownValues: true,
		Schema: Schema{
			stringFieldSchema(),
			nestedFieldSchema(),
		},
		CSVOptions: CSVOptions{
			Quote:               hyphen,
			FieldDelimiter:      "\t",
			SkipLeadingRows:     8,
			AllowJaggedRows:     true,
			AllowQuotedNewlines: true,
			Encoding:            UTF_8,
		},
	}
)

func TestFileConfigPopulateLoadConfig(t *testing.T) {
	want := &bq.JobConfigurationLoad{
		SourceFormat:        "CSV",
		FieldDelimiter:      "\t",
		SkipLeadingRows:     8,
		AllowJaggedRows:     true,
		AllowQuotedNewlines: true,
		Autodetect:          true,
		Encoding:            "UTF-8",
		MaxBadRecords:       7,
		IgnoreUnknownValues: true,
		Schema: &bq.TableSchema{
			Fields: []*bq.TableFieldSchema{
				bqStringFieldSchema(),
				bqNestedFieldSchema(),
			}},
		Quote: &hyphen,
	}
	got := &bq.JobConfigurationLoad{}
	fc.populateLoadConfig(got)
	if !testutil.Equal(got, want) {
		t.Errorf("got:\n%v\nwant:\n%v", pretty.Value(got), pretty.Value(want))
	}
}

func TestFileConfigPopulateExternalDataConfig(t *testing.T) {
	got := &bq.ExternalDataConfiguration{}
	fc.populateExternalDataConfig(got)

	want := &bq.ExternalDataConfiguration{
		SourceFormat:        "CSV",
		Autodetect:          true,
		MaxBadRecords:       7,
		IgnoreUnknownValues: true,
		Schema: &bq.TableSchema{
			Fields: []*bq.TableFieldSchema{
				bqStringFieldSchema(),
				bqNestedFieldSchema(),
			}},
		CsvOptions: &bq.CsvOptions{
			AllowJaggedRows:     true,
			AllowQuotedNewlines: true,
			Encoding:            "UTF-8",
			FieldDelimiter:      "\t",
			Quote:               &hyphen,
			SkipLeadingRows:     8,
		},
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("got=-, want=+:\n%s", diff)
	}
}
