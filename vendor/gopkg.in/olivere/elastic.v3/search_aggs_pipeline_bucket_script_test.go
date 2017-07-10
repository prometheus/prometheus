// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestBucketScriptAggregation(t *testing.T) {
	agg := NewBucketScriptAggregation().
		AddBucketsPath("tShirtSales", "t-shirts>sales").
		AddBucketsPath("totalSales", "total_sales").
		Script(NewScript("tShirtSales / totalSales * 100"))
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"bucket_script":{"buckets_path":{"tShirtSales":"t-shirts\u003esales","totalSales":"total_sales"},"script":"tShirtSales / totalSales * 100"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
