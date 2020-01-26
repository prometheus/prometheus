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

	"cloud.google.com/go/internal/pretty"
	"cloud.google.com/go/internal/testutil"
)

func TestExternalDataConfig(t *testing.T) {
	// Round-trip of ExternalDataConfig to underlying representation.
	for i, want := range []*ExternalDataConfig{
		{
			SourceFormat:        CSV,
			SourceURIs:          []string{"uri"},
			Schema:              Schema{{Name: "n", Type: IntegerFieldType}},
			AutoDetect:          true,
			Compression:         Gzip,
			IgnoreUnknownValues: true,
			MaxBadRecords:       17,
			Options: &CSVOptions{
				AllowJaggedRows:     true,
				AllowQuotedNewlines: true,
				Encoding:            UTF_8,
				FieldDelimiter:      "f",
				Quote:               "q",
				SkipLeadingRows:     3,
			},
		},
		{
			SourceFormat: GoogleSheets,
			Options:      &GoogleSheetsOptions{SkipLeadingRows: 4},
		},
		{
			SourceFormat: Bigtable,
			Options: &BigtableOptions{
				IgnoreUnspecifiedColumnFamilies: true,
				ReadRowkeyAsString:              true,
				ColumnFamilies: []*BigtableColumnFamily{
					{
						FamilyID:       "f1",
						Encoding:       "TEXT",
						OnlyReadLatest: true,
						Type:           "FLOAT",
						Columns: []*BigtableColumn{
							{
								Qualifier:      "valid-utf-8",
								FieldName:      "fn",
								OnlyReadLatest: true,
								Encoding:       "BINARY",
								Type:           "STRING",
							},
						},
					},
				},
			},
		},
	} {
		q := want.toBQ()
		got, err := bqToExternalDataConfig(&q)
		if err != nil {
			t.Fatal(err)
		}
		if diff := testutil.Diff(got, want); diff != "" {
			t.Errorf("#%d: got=-, want=+:\n%s", i, diff)
		}
	}
}

func TestQuote(t *testing.T) {
	ptr := func(s string) *string { return &s }

	for _, test := range []struct {
		quote string
		force bool
		want  *string
	}{
		{"", false, nil},
		{"", true, ptr("")},
		{"-", false, ptr("-")},
		{"-", true, ptr("")},
	} {
		o := CSVOptions{
			Quote:          test.quote,
			ForceZeroQuote: test.force,
		}
		got := o.quote()
		if (got == nil) != (test.want == nil) {
			t.Errorf("%+v\ngot %v\nwant %v", test, pretty.Value(got), pretty.Value(test.want))
		}
		if got != nil && test.want != nil && *got != *test.want {
			t.Errorf("%+v: got %q, want %q", test, *got, *test.want)
		}
	}
}

func TestQualifier(t *testing.T) {
	b := BigtableColumn{Qualifier: "a"}
	q := b.toBQ()
	if q.QualifierString != b.Qualifier || q.QualifierEncoded != "" {
		t.Errorf("got (%q, %q), want (%q, %q)",
			q.QualifierString, q.QualifierEncoded, b.Qualifier, "")
	}
	b2, err := bqToBigtableColumn(q)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := b2.Qualifier, b.Qualifier; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	const (
		invalidUTF8    = "\xDF\xFF"
		invalidEncoded = "3/8"
	)
	b = BigtableColumn{Qualifier: invalidUTF8}
	q = b.toBQ()
	if q.QualifierString != "" || q.QualifierEncoded != invalidEncoded {
		t.Errorf("got (%q, %q), want (%q, %q)",
			q.QualifierString, "", b.Qualifier, invalidEncoded)
	}
	b2, err = bqToBigtableColumn(q)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := b2.Qualifier, b.Qualifier; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
