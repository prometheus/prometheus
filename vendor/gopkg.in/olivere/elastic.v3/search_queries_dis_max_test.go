// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestDisMaxQuery(t *testing.T) {
	q := NewDisMaxQuery()
	q = q.Query(NewTermQuery("age", 34), NewTermQuery("age", 35)).Boost(1.2).TieBreaker(0.7)
	src, err := q.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"dis_max":{"boost":1.2,"queries":[{"term":{"age":34}},{"term":{"age":35}}],"tie_breaker":0.7}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
