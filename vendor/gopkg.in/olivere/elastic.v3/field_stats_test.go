// Copyright 2012-present Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"net/url"
	"reflect"
	"sort"
	"testing"
)

func TestFieldStatsURLs(t *testing.T) {
	tests := []struct {
		Service        *FieldStatsService
		ExpectedPath   string
		ExpectedParams url.Values
	}{
		{
			Service:        &FieldStatsService{},
			ExpectedPath:   "/_field_stats",
			ExpectedParams: url.Values{},
		},
		{
			Service: &FieldStatsService{
				level: FieldStatsClusterLevel,
			},
			ExpectedPath:   "/_field_stats",
			ExpectedParams: url.Values{"level": []string{FieldStatsClusterLevel}},
		},
		{
			Service: &FieldStatsService{
				level: FieldStatsIndicesLevel,
			},
			ExpectedPath:   "/_field_stats",
			ExpectedParams: url.Values{"level": []string{FieldStatsIndicesLevel}},
		},
		{
			Service: &FieldStatsService{
				level: FieldStatsClusterLevel,
				index: []string{"index1"},
			},
			ExpectedPath:   "/index1/_field_stats",
			ExpectedParams: url.Values{"level": []string{FieldStatsClusterLevel}},
		},
		{
			Service: &FieldStatsService{
				level: FieldStatsIndicesLevel,
				index: []string{"index1", "index2"},
			},
			ExpectedPath:   "/index1%2Cindex2/_field_stats",
			ExpectedParams: url.Values{"level": []string{FieldStatsIndicesLevel}},
		},
		{
			Service: &FieldStatsService{
				level: FieldStatsIndicesLevel,
				index: []string{"index_*"},
			},
			ExpectedPath:   "/index_%2A/_field_stats",
			ExpectedParams: url.Values{"level": []string{FieldStatsIndicesLevel}},
		},
	}

	for _, test := range tests {
		gotPath, gotParams, err := test.Service.buildURL()
		if err != nil {
			t.Fatalf("expected no error; got: %v", err)
		}
		if gotPath != test.ExpectedPath {
			t.Errorf("expected URL path = %q; got: %q", test.ExpectedPath, gotPath)
		}
		if gotParams.Encode() != test.ExpectedParams.Encode() {
			t.Errorf("expected URL params = %v; got: %v", test.ExpectedParams, gotParams)
		}
	}
}

func TestFieldStatsValidate(t *testing.T) {
	tests := []struct {
		Service *FieldStatsService
		Valid   bool
	}{
		{
			Service: &FieldStatsService{},
			Valid:   true,
		},
		{
			Service: &FieldStatsService{
				fields: []string{"field"},
			},
			Valid: true,
		},
		{
			Service: &FieldStatsService{
				bodyJson: &FieldStatsRequest{
					Fields: []string{"field"},
				},
			},
			Valid: true,
		},
		{
			Service: &FieldStatsService{
				level: FieldStatsClusterLevel,
				bodyJson: &FieldStatsRequest{
					Fields: []string{"field"},
				},
			},
			Valid: true,
		},
		{
			Service: &FieldStatsService{
				level: FieldStatsIndicesLevel,
				bodyJson: &FieldStatsRequest{
					Fields: []string{"field"},
				},
			},
			Valid: true,
		},
		{
			Service: &FieldStatsService{
				level: "random",
			},
			Valid: false,
		},
	}

	for _, test := range tests {
		err := test.Service.Validate()
		isValid := err == nil
		if isValid != test.Valid {
			t.Errorf("expected validity to be %v, got %v", test.Valid, isValid)
		}
	}
}

func TestFieldStatsRequestSerialize(t *testing.T) {
	req := &FieldStatsRequest{
		Fields: []string{"creation_date", "answer_count"},
		IndexConstraints: map[string]*FieldStatsConstraints{
			"creation_date": &FieldStatsConstraints{
				Min: &FieldStatsComparison{Gte: "2014-01-01T00:00:00.000Z"},
				Max: &FieldStatsComparison{Lt: "2015-01-01T10:00:00.000Z"},
			},
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"fields":["creation_date","answer_count"],"index_constraints":{"creation_date":{"min_value":{"gte":"2014-01-01T00:00:00.000Z"},"max_value":{"lt":"2015-01-01T10:00:00.000Z"}}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestFieldStatsRequestDeserialize(t *testing.T) {
	body := `{
		"fields" : ["creation_date", "answer_count"],
   	"index_constraints" : {
      "creation_date" : {
         "min_value" : {
            "gte" : "2014-01-01T00:00:00.000Z"
         },
         "max_value" : {
            "lt" : "2015-01-01T10:00:00.000Z"
         }
      }
   	}
	}`

	var request FieldStatsRequest
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Errorf("unexpected error during unmarshalling: %v", err)
	}

	sort.Sort(lexicographically{request.Fields})

	expectedFields := []string{"answer_count", "creation_date"}
	if !reflect.DeepEqual(request.Fields, expectedFields) {
		t.Errorf("expected fields to be %v, got %v", expectedFields, request.Fields)
	}

	constraints, ok := request.IndexConstraints["creation_date"]
	if !ok {
		t.Errorf("expected field creation_date, didn't find it!")
	}
	if constraints.Min.Lt != nil {
		t.Errorf("expected min value less than constraint to be empty, got %v", constraints.Min.Lt)
	}
	if constraints.Min.Gte != "2014-01-01T00:00:00.000Z" {
		t.Errorf("expected min value >= %v, found %v", "2014-01-01T00:00:00.000Z", constraints.Min.Gte)
	}
	if constraints.Max.Lt != "2015-01-01T10:00:00.000Z" {
		t.Errorf("expected max value < %v, found %v", "2015-01-01T10:00:00.000Z", constraints.Max.Lt)
	}
}

func TestFieldStatsResponseUnmarshalling(t *testing.T) {
	clusterStats := `{
		 "_shards": {
				"total": 1,
				"successful": 1,
				"failed": 0
		 },
		 "indices": {
				"_all": {
					 "fields": {
							"creation_date": {
								 "max_doc": 1326564,
								 "doc_count": 564633,
								 "density": 42,
								 "sum_doc_freq": 2258532,
								 "sum_total_term_freq": -1,
								 "min_value_as_string": "2008-08-01T16:37:51.513Z",
								 "max_value_as_string": "2013-06-02T03:23:11.593Z"
							},
							"answer_count": {
								 "max_doc": 1326564,
								 "doc_count": 139885,
								 "density": 10,
								 "sum_doc_freq": 559540,
								 "sum_total_term_freq": -1,
								 "min_value_as_string": "0",
								 "max_value_as_string": "160"
							}
					 }
				}
		 }
	}`

	var response FieldStatsResponse
	if err := json.Unmarshal([]byte(clusterStats), &response); err != nil {
		t.Errorf("unexpected error during unmarshalling: %v", err)
	}

	stats, ok := response.Indices["_all"]
	if !ok {
		t.Errorf("expected _all to be in the indices map, didn't find it")
	}

	fieldStats, ok := stats.Fields["creation_date"]
	if !ok {
		t.Errorf("expected creation_date to be in the fields map, didn't find it")
	}
	if fieldStats.MinValueAsString != "2008-08-01T16:37:51.513Z" {
		t.Errorf("expected creation_date min value to be %v, got %v", "2008-08-01T16:37:51.513Z", fieldStats.MinValueAsString)
	}
}

type lexicographically struct {
	strings []string
}

func (l lexicographically) Len() int {
	return len(l.strings)
}

func (l lexicographically) Less(i, j int) bool {
	return l.strings[i] < l.strings[j]
}

func (l lexicographically) Swap(i, j int) {
	l.strings[i], l.strings[j] = l.strings[j], l.strings[i]
}
