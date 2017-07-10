// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import "testing"

func TestAggsIntegrationAvgBucket(t *testing.T) {
	//client := setupTestClientAndCreateIndexAndAddDocs(t, SetTraceLog(log.New(os.Stdout, "", log.LstdFlags)))
	client := setupTestClientAndCreateIndexAndAddDocs(t)

	esversion, err := client.ElasticsearchVersion(DefaultURL)
	if err != nil {
		t.Fatal(err)
	}

	if esversion < "2.0" {
		t.Skipf("Elasticsearch %s does not have pipeline aggregations.", esversion)
		return
	}

	// Match all should return all documents
	builder := client.Search().
		Index(testIndexName).
		Type("order").
		Query(NewMatchAllQuery()).
		Pretty(true)
	h := NewDateHistogramAggregation().Field("time").Interval("month")
	h = h.SubAggregation("sales", NewSumAggregation().Field("price"))
	builder = builder.Aggregation("sales_per_month", h)
	builder = builder.Aggregation("avg_monthly_sales", NewAvgBucketAggregation().BucketsPath("sales_per_month>sales"))

	res, err := builder.Do()
	if err != nil {
		t.Fatal(err)
	}
	if res.Hits == nil {
		t.Errorf("expected Hits != nil; got: nil")
	}

	aggs := res.Aggregations
	if aggs == nil {
		t.Fatal("expected aggregations != nil; got: nil")
	}

	agg, found := aggs.AvgBucket("avg_monthly_sales")
	if !found {
		t.Fatal("expected avg_monthly_sales aggregation")
	}
	if agg == nil {
		t.Fatal("expected avg_monthly_sales aggregation")
	}
	if agg.Value == nil {
		t.Fatal("expected avg_monthly_sales.value != nil")
	}
	if got, want := *agg.Value, float64(939.2); got != want {
		t.Fatalf("expected avg_monthly_sales.value=%v; got: %v", want, got)
	}
}

func TestAggsIntegrationDerivative(t *testing.T) {
	//client := setupTestClientAndCreateIndexAndAddDocs(t, SetTraceLog(log.New(os.Stdout, "", log.LstdFlags)))
	client := setupTestClientAndCreateIndexAndAddDocs(t)

	esversion, err := client.ElasticsearchVersion(DefaultURL)
	if err != nil {
		t.Fatal(err)
	}

	if esversion < "2.0" {
		t.Skipf("Elasticsearch %s does not have pipeline aggregations.", esversion)
		return
	}

	// Match all should return all documents
	builder := client.Search().
		Index(testIndexName).
		Type("order").
		Query(NewMatchAllQuery()).
		Pretty(true)
	h := NewDateHistogramAggregation().Field("time").Interval("month")
	h = h.SubAggregation("sales", NewSumAggregation().Field("price"))
	h = h.SubAggregation("sales_deriv", NewDerivativeAggregation().BucketsPath("sales"))
	builder = builder.Aggregation("sales_per_month", h)

	res, err := builder.Do()
	if err != nil {
		t.Fatal(err)
	}
	if res.Hits == nil {
		t.Errorf("expected Hits != nil; got: nil")
	}

	aggs := res.Aggregations
	if aggs == nil {
		t.Fatal("expected aggregations != nil; got: nil")
	}

	agg, found := aggs.DateHistogram("sales_per_month")
	if !found {
		t.Fatal("expected sales_per_month aggregation")
	}
	if agg == nil {
		t.Fatal("expected sales_per_month aggregation")
	}
	if got, want := len(agg.Buckets), 6; got != want {
		t.Fatalf("expected %d buckets; got: %d", want, got)
	}

	if got, want := agg.Buckets[0].DocCount, int64(1); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[1].DocCount, int64(0); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[2].DocCount, int64(1); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[3].DocCount, int64(3); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[4].DocCount, int64(1); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[5].DocCount, int64(2); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}

	d, found := agg.Buckets[0].Derivative("sales_deriv")
	if found {
		t.Fatal("expected no sales_deriv aggregation")
	}
	if d != nil {
		t.Fatal("expected no sales_deriv aggregation")
	}

	d, found = agg.Buckets[1].Derivative("sales_deriv")
	if !found {
		t.Fatal("expected sales_deriv aggregation")
	}
	if d == nil {
		t.Fatal("expected sales_deriv aggregation")
	}
	if d.Value != nil {
		t.Fatal("expected sales_deriv value == nil")
	}

	d, found = agg.Buckets[2].Derivative("sales_deriv")
	if !found {
		t.Fatal("expected sales_deriv aggregation")
	}
	if d == nil {
		t.Fatal("expected sales_deriv aggregation")
	}
	if d.Value != nil {
		t.Fatal("expected sales_deriv value == nil")
	}

	d, found = agg.Buckets[3].Derivative("sales_deriv")
	if !found {
		t.Fatal("expected sales_deriv aggregation")
	}
	if d == nil {
		t.Fatal("expected sales_deriv aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected sales_deriv value != nil")
	}
	if got, want := *d.Value, float64(2348.0); got != want {
		t.Fatalf("expected sales_deriv.value=%v; got: %v", want, got)
	}

	d, found = agg.Buckets[4].Derivative("sales_deriv")
	if !found {
		t.Fatal("expected sales_deriv aggregation")
	}
	if d == nil {
		t.Fatal("expected sales_deriv aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected sales_deriv value != nil")
	}
	if got, want := *d.Value, float64(-1658.0); got != want {
		t.Fatalf("expected sales_deriv.value=%v; got: %v", want, got)
	}

	d, found = agg.Buckets[5].Derivative("sales_deriv")
	if !found {
		t.Fatal("expected sales_deriv aggregation")
	}
	if d == nil {
		t.Fatal("expected sales_deriv aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected sales_deriv value != nil")
	}
	if got, want := *d.Value, float64(-722.0); got != want {
		t.Fatalf("expected sales_deriv.value=%v; got: %v", want, got)
	}
}

func TestAggsIntegrationMaxBucket(t *testing.T) {
	//client := setupTestClientAndCreateIndexAndAddDocs(t, SetTraceLog(log.New(os.Stdout, "", log.LstdFlags)))
	client := setupTestClientAndCreateIndexAndAddDocs(t)

	esversion, err := client.ElasticsearchVersion(DefaultURL)
	if err != nil {
		t.Fatal(err)
	}

	if esversion < "2.0" {
		t.Skipf("Elasticsearch %s does not have pipeline aggregations.", esversion)
		return
	}

	// Match all should return all documents
	builder := client.Search().
		Index(testIndexName).
		Type("order").
		Query(NewMatchAllQuery()).
		Pretty(true)
	h := NewDateHistogramAggregation().Field("time").Interval("month")
	h = h.SubAggregation("sales", NewSumAggregation().Field("price"))
	builder = builder.Aggregation("sales_per_month", h)
	builder = builder.Aggregation("max_monthly_sales", NewMaxBucketAggregation().BucketsPath("sales_per_month>sales"))

	res, err := builder.Do()
	if err != nil {
		t.Fatal(err)
	}
	if res.Hits == nil {
		t.Errorf("expected Hits != nil; got: nil")
	}

	aggs := res.Aggregations
	if aggs == nil {
		t.Fatal("expected aggregations != nil; got: nil")
	}

	agg, found := aggs.MaxBucket("max_monthly_sales")
	if !found {
		t.Fatal("expected max_monthly_sales aggregation")
	}
	if agg == nil {
		t.Fatal("expected max_monthly_sales aggregation")
	}
	if got, want := len(agg.Keys), 1; got != want {
		t.Fatalf("expected len(max_monthly_sales.keys)=%d; got: %d", want, got)
	}
	if got, want := agg.Keys[0], "2015-04-01"; got != want {
		t.Fatalf("expected max_monthly_sales.keys[0]=%v; got: %v", want, got)
	}
	if agg.Value == nil {
		t.Fatal("expected max_monthly_sales.value != nil")
	}
	if got, want := *agg.Value, float64(2448); got != want {
		t.Fatalf("expected max_monthly_sales.value=%v; got: %v", want, got)
	}
}

func TestAggsIntegrationMinBucket(t *testing.T) {
	//client := setupTestClientAndCreateIndexAndAddDocs(t, SetTraceLog(log.New(os.Stdout, "", log.LstdFlags)))
	client := setupTestClientAndCreateIndexAndAddDocs(t)

	esversion, err := client.ElasticsearchVersion(DefaultURL)
	if err != nil {
		t.Fatal(err)
	}

	if esversion < "2.0" {
		t.Skipf("Elasticsearch %s does not have pipeline aggregations.", esversion)
		return
	}

	// Match all should return all documents
	builder := client.Search().
		Index(testIndexName).
		Type("order").
		Query(NewMatchAllQuery()).
		Pretty(true)
	h := NewDateHistogramAggregation().Field("time").Interval("month")
	h = h.SubAggregation("sales", NewSumAggregation().Field("price"))
	builder = builder.Aggregation("sales_per_month", h)
	builder = builder.Aggregation("min_monthly_sales", NewMinBucketAggregation().BucketsPath("sales_per_month>sales"))

	res, err := builder.Do()
	if err != nil {
		t.Fatal(err)
	}
	if res.Hits == nil {
		t.Errorf("expected Hits != nil; got: nil")
	}

	aggs := res.Aggregations
	if aggs == nil {
		t.Fatal("expected aggregations != nil; got: nil")
	}

	agg, found := aggs.MinBucket("min_monthly_sales")
	if !found {
		t.Fatal("expected min_monthly_sales aggregation")
	}
	if agg == nil {
		t.Fatal("expected min_monthly_sales aggregation")
	}
	if got, want := len(agg.Keys), 1; got != want {
		t.Fatalf("expected len(min_monthly_sales.keys)=%d; got: %d", want, got)
	}
	if got, want := agg.Keys[0], "2015-06-01"; got != want {
		t.Fatalf("expected min_monthly_sales.keys[0]=%v; got: %v", want, got)
	}
	if agg.Value == nil {
		t.Fatal("expected min_monthly_sales.value != nil")
	}
	if got, want := *agg.Value, float64(68); got != want {
		t.Fatalf("expected min_monthly_sales.value=%v; got: %v", want, got)
	}
}

func TestAggsIntegrationSumBucket(t *testing.T) {
	//client := setupTestClientAndCreateIndexAndAddDocs(t, SetTraceLog(log.New(os.Stdout, "", log.LstdFlags)))
	client := setupTestClientAndCreateIndexAndAddDocs(t)

	esversion, err := client.ElasticsearchVersion(DefaultURL)
	if err != nil {
		t.Fatal(err)
	}

	if esversion < "2.0" {
		t.Skipf("Elasticsearch %s does not have pipeline aggregations.", esversion)
		return
	}

	// Match all should return all documents
	builder := client.Search().
		Index(testIndexName).
		Type("order").
		Query(NewMatchAllQuery()).
		Pretty(true)
	h := NewDateHistogramAggregation().Field("time").Interval("month")
	h = h.SubAggregation("sales", NewSumAggregation().Field("price"))
	builder = builder.Aggregation("sales_per_month", h)
	builder = builder.Aggregation("sum_monthly_sales", NewSumBucketAggregation().BucketsPath("sales_per_month>sales"))

	res, err := builder.Do()
	if err != nil {
		t.Fatal(err)
	}
	if res.Hits == nil {
		t.Errorf("expected Hits != nil; got: nil")
	}

	aggs := res.Aggregations
	if aggs == nil {
		t.Fatal("expected aggregations != nil; got: nil")
	}

	agg, found := aggs.SumBucket("sum_monthly_sales")
	if !found {
		t.Fatal("expected sum_monthly_sales aggregation")
	}
	if agg == nil {
		t.Fatal("expected sum_monthly_sales aggregation")
	}
	if agg.Value == nil {
		t.Fatal("expected sum_monthly_sales.value != nil")
	}
	if got, want := *agg.Value, float64(4696.0); got != want {
		t.Fatalf("expected sum_monthly_sales.value=%v; got: %v", want, got)
	}
}

func TestAggsIntegrationMovAvg(t *testing.T) {
	//client := setupTestClientAndCreateIndexAndAddDocs(t, SetTraceLog(log.New(os.Stdout, "", log.LstdFlags)))
	client := setupTestClientAndCreateIndexAndAddDocs(t)

	esversion, err := client.ElasticsearchVersion(DefaultURL)
	if err != nil {
		t.Fatal(err)
	}

	if esversion < "2.0" {
		t.Skipf("Elasticsearch %s does not have pipeline aggregations.", esversion)
		return
	}

	// Match all should return all documents
	builder := client.Search().
		Index(testIndexName).
		Type("order").
		Query(NewMatchAllQuery()).
		Pretty(true)
	h := NewDateHistogramAggregation().Field("time").Interval("month")
	h = h.SubAggregation("the_sum", NewSumAggregation().Field("price"))
	h = h.SubAggregation("the_movavg", NewMovAvgAggregation().BucketsPath("the_sum"))
	builder = builder.Aggregation("my_date_histo", h)

	res, err := builder.Do()
	if err != nil {
		t.Fatal(err)
	}
	if res.Hits == nil {
		t.Errorf("expected Hits != nil; got: nil")
	}

	aggs := res.Aggregations
	if aggs == nil {
		t.Fatal("expected aggregations != nil; got: nil")
	}

	agg, found := aggs.DateHistogram("my_date_histo")
	if !found {
		t.Fatal("expected sum_monthly_sales aggregation")
	}
	if agg == nil {
		t.Fatal("expected sum_monthly_sales aggregation")
	}
	if got, want := len(agg.Buckets), 6; got != want {
		t.Fatalf("expected %d buckets; got: %d", want, got)
	}

	d, found := agg.Buckets[0].MovAvg("the_movavg")
	if found {
		t.Fatal("expected no the_movavg aggregation")
	}
	if d != nil {
		t.Fatal("expected no the_movavg aggregation")
	}

	d, found = agg.Buckets[1].MovAvg("the_movavg")
	if found {
		t.Fatal("expected no the_movavg aggregation")
	}
	if d != nil {
		t.Fatal("expected no the_movavg aggregation")
	}

	d, found = agg.Buckets[2].MovAvg("the_movavg")
	if !found {
		t.Fatal("expected the_movavg aggregation")
	}
	if d == nil {
		t.Fatal("expected the_movavg aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected the_movavg value")
	}
	if got, want := *d.Value, float64(1290.0); got != want {
		t.Fatalf("expected %v buckets; got: %v", want, got)
	}

	d, found = agg.Buckets[3].MovAvg("the_movavg")
	if !found {
		t.Fatal("expected the_movavg aggregation")
	}
	if d == nil {
		t.Fatal("expected the_movavg aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected the_movavg value")
	}
	if got, want := *d.Value, float64(695.0); got != want {
		t.Fatalf("expected %v buckets; got: %v", want, got)
	}

	d, found = agg.Buckets[4].MovAvg("the_movavg")
	if !found {
		t.Fatal("expected the_movavg aggregation")
	}
	if d == nil {
		t.Fatal("expected the_movavg aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected the_movavg value")
	}
	if got, want := *d.Value, float64(1279.3333333333333); got != want {
		t.Fatalf("expected %v buckets; got: %v", want, got)
	}

	d, found = agg.Buckets[5].MovAvg("the_movavg")
	if !found {
		t.Fatal("expected the_movavg aggregation")
	}
	if d == nil {
		t.Fatal("expected the_movavg aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected the_movavg value")
	}
	if got, want := *d.Value, float64(1157.0); got != want {
		t.Fatalf("expected %v buckets; got: %v", want, got)
	}
}

func TestAggsIntegrationCumulativeSum(t *testing.T) {
	//client := setupTestClientAndCreateIndexAndAddDocs(t, SetTraceLog(log.New(os.Stdout, "", log.LstdFlags)))
	client := setupTestClientAndCreateIndexAndAddDocs(t)

	esversion, err := client.ElasticsearchVersion(DefaultURL)
	if err != nil {
		t.Fatal(err)
	}

	if esversion < "2.0" {
		t.Skipf("Elasticsearch %s does not have pipeline aggregations.", esversion)
		return
	}

	// Match all should return all documents
	builder := client.Search().
		Index(testIndexName).
		Type("order").
		Query(NewMatchAllQuery()).
		Pretty(true)
	h := NewDateHistogramAggregation().Field("time").Interval("month")
	h = h.SubAggregation("sales", NewSumAggregation().Field("price"))
	h = h.SubAggregation("cumulative_sales", NewCumulativeSumAggregation().BucketsPath("sales"))
	builder = builder.Aggregation("sales_per_month", h)

	res, err := builder.Do()
	if err != nil {
		t.Fatal(err)
	}
	if res.Hits == nil {
		t.Errorf("expected Hits != nil; got: nil")
	}

	aggs := res.Aggregations
	if aggs == nil {
		t.Fatal("expected aggregations != nil; got: nil")
	}

	agg, found := aggs.DateHistogram("sales_per_month")
	if !found {
		t.Fatal("expected sales_per_month aggregation")
	}
	if agg == nil {
		t.Fatal("expected sales_per_month aggregation")
	}
	if got, want := len(agg.Buckets), 6; got != want {
		t.Fatalf("expected %d buckets; got: %d", want, got)
	}

	if got, want := agg.Buckets[0].DocCount, int64(1); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[1].DocCount, int64(0); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[2].DocCount, int64(1); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[3].DocCount, int64(3); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[4].DocCount, int64(1); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[5].DocCount, int64(2); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}

	d, found := agg.Buckets[0].CumulativeSum("cumulative_sales")
	if !found {
		t.Fatal("expected cumulative_sales aggregation")
	}
	if d == nil {
		t.Fatal("expected cumulative_sales aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected cumulative_sales value != nil")
	}
	if got, want := *d.Value, float64(1290.0); got != want {
		t.Fatalf("expected cumulative_sales.value=%v; got: %v", want, got)
	}

	d, found = agg.Buckets[1].CumulativeSum("cumulative_sales")
	if !found {
		t.Fatal("expected cumulative_sales aggregation")
	}
	if d == nil {
		t.Fatal("expected cumulative_sales aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected cumulative_sales value != nil")
	}
	if got, want := *d.Value, float64(1290.0); got != want {
		t.Fatalf("expected cumulative_sales.value=%v; got: %v", want, got)
	}

	d, found = agg.Buckets[2].CumulativeSum("cumulative_sales")
	if !found {
		t.Fatal("expected cumulative_sales aggregation")
	}
	if d == nil {
		t.Fatal("expected cumulative_sales aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected cumulative_sales value != nil")
	}
	if got, want := *d.Value, float64(1390.0); got != want {
		t.Fatalf("expected cumulative_sales.value=%v; got: %v", want, got)
	}

	d, found = agg.Buckets[3].CumulativeSum("cumulative_sales")
	if !found {
		t.Fatal("expected cumulative_sales aggregation")
	}
	if d == nil {
		t.Fatal("expected cumulative_sales aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected cumulative_sales value != nil")
	}
	if got, want := *d.Value, float64(3838.0); got != want {
		t.Fatalf("expected cumulative_sales.value=%v; got: %v", want, got)
	}

	d, found = agg.Buckets[4].CumulativeSum("cumulative_sales")
	if !found {
		t.Fatal("expected cumulative_sales aggregation")
	}
	if d == nil {
		t.Fatal("expected cumulative_sales aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected cumulative_sales value != nil")
	}
	if got, want := *d.Value, float64(4628.0); got != want {
		t.Fatalf("expected cumulative_sales.value=%v; got: %v", want, got)
	}

	d, found = agg.Buckets[5].CumulativeSum("cumulative_sales")
	if !found {
		t.Fatal("expected cumulative_sales aggregation")
	}
	if d == nil {
		t.Fatal("expected cumulative_sales aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected cumulative_sales value != nil")
	}
	if got, want := *d.Value, float64(4696.0); got != want {
		t.Fatalf("expected cumulative_sales.value=%v; got: %v", want, got)
	}
}

func TestAggsIntegrationBucketScript(t *testing.T) {
	//client := setupTestClientAndCreateIndexAndAddDocs(t, SetTraceLog(log.New(os.Stdout, "", log.LstdFlags)))
	client := setupTestClientAndCreateIndexAndAddDocs(t)

	esversion, err := client.ElasticsearchVersion(DefaultURL)
	if err != nil {
		t.Fatal(err)
	}

	if esversion < "2.0" {
		t.Skipf("Elasticsearch %s does not have pipeline aggregations.", esversion)
		return
	}

	// Match all should return all documents
	builder := client.Search().
		Index(testIndexName).
		Type("order").
		Query(NewMatchAllQuery()).
		Pretty(true)
	h := NewDateHistogramAggregation().Field("time").Interval("month")
	h = h.SubAggregation("total_sales", NewSumAggregation().Field("price"))
	appleFilter := NewFilterAggregation().Filter(NewTermQuery("manufacturer", "Apple"))
	appleFilter = appleFilter.SubAggregation("sales", NewSumAggregation().Field("price"))
	h = h.SubAggregation("apple_sales", appleFilter)
	h = h.SubAggregation("apple_percentage",
		NewBucketScriptAggregation().
			GapPolicy("insert_zeros").
			AddBucketsPath("appleSales", "apple_sales>sales").
			AddBucketsPath("totalSales", "total_sales").
			Script(NewScript("appleSales / totalSales * 100")))
	builder = builder.Aggregation("sales_per_month", h)

	res, err := builder.Do()
	if err != nil {
		t.Fatalf("%v (maybe scripting is disabled?)", err)
	}
	if res.Hits == nil {
		t.Errorf("expected Hits != nil; got: nil")
	}

	aggs := res.Aggregations
	if aggs == nil {
		t.Fatal("expected aggregations != nil; got: nil")
	}

	agg, found := aggs.DateHistogram("sales_per_month")
	if !found {
		t.Fatal("expected sales_per_month aggregation")
	}
	if agg == nil {
		t.Fatal("expected sales_per_month aggregation")
	}
	if got, want := len(agg.Buckets), 6; got != want {
		t.Fatalf("expected %d buckets; got: %d", want, got)
	}

	if got, want := agg.Buckets[0].DocCount, int64(1); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[1].DocCount, int64(0); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[2].DocCount, int64(1); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[3].DocCount, int64(3); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[4].DocCount, int64(1); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[5].DocCount, int64(2); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}

	d, found := agg.Buckets[0].BucketScript("apple_percentage")
	if !found {
		t.Fatal("expected apple_percentage aggregation")
	}
	if d == nil {
		t.Fatal("expected apple_percentage aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected apple_percentage value != nil")
	}
	if got, want := *d.Value, float64(100.0); got != want {
		t.Fatalf("expected apple_percentage.value=%v; got: %v", want, got)
	}

	d, found = agg.Buckets[1].BucketScript("apple_percentage")
	if !found {
		t.Fatal("expected apple_percentage aggregation")
	}
	if d == nil {
		t.Fatal("expected apple_percentage aggregation")
	}
	if d.Value != nil {
		t.Fatal("expected apple_percentage value == nil")
	}

	d, found = agg.Buckets[2].BucketScript("apple_percentage")
	if !found {
		t.Fatal("expected apple_percentage aggregation")
	}
	if d == nil {
		t.Fatal("expected apple_percentage aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected apple_percentage value != nil")
	}
	if got, want := *d.Value, float64(0.0); got != want {
		t.Fatalf("expected apple_percentage.value=%v; got: %v", want, got)
	}

	d, found = agg.Buckets[3].BucketScript("apple_percentage")
	if !found {
		t.Fatal("expected apple_percentage aggregation")
	}
	if d == nil {
		t.Fatal("expected apple_percentage aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected apple_percentage value != nil")
	}
	if got, want := *d.Value, float64(34.64052287581699); got != want {
		t.Fatalf("expected apple_percentage.value=%v; got: %v", want, got)
	}

	d, found = agg.Buckets[4].BucketScript("apple_percentage")
	if !found {
		t.Fatal("expected apple_percentage aggregation")
	}
	if d == nil {
		t.Fatal("expected apple_percentage aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected apple_percentage value != nil")
	}
	if got, want := *d.Value, float64(0.0); got != want {
		t.Fatalf("expected apple_percentage.value=%v; got: %v", want, got)
	}

	d, found = agg.Buckets[5].BucketScript("apple_percentage")
	if !found {
		t.Fatal("expected apple_percentage aggregation")
	}
	if d == nil {
		t.Fatal("expected apple_percentage aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected apple_percentage value != nil")
	}
	if got, want := *d.Value, float64(0.0); got != want {
		t.Fatalf("expected apple_percentage.value=%v; got: %v", want, got)
	}
}

func TestAggsIntegrationBucketSelector(t *testing.T) {
	//client := setupTestClientAndCreateIndexAndAddDocs(t, SetTraceLog(log.New(os.Stdout, "", log.LstdFlags)))
	client := setupTestClientAndCreateIndexAndAddDocs(t)

	esversion, err := client.ElasticsearchVersion(DefaultURL)
	if err != nil {
		t.Fatal(err)
	}

	if esversion < "2.0" {
		t.Skipf("Elasticsearch %s does not have pipeline aggregations.", esversion)
		return
	}

	// Match all should return all documents
	builder := client.Search().
		Index(testIndexName).
		Type("order").
		Query(NewMatchAllQuery()).
		Pretty(true)
	h := NewDateHistogramAggregation().Field("time").Interval("month")
	h = h.SubAggregation("total_sales", NewSumAggregation().Field("price"))
	h = h.SubAggregation("sales_bucket_filter",
		NewBucketSelectorAggregation().
			AddBucketsPath("totalSales", "total_sales").
			Script(NewScript("totalSales <= 100")))
	builder = builder.Aggregation("sales_per_month", h)

	res, err := builder.Do()
	if err != nil {
		t.Fatalf("%v (maybe scripting is disabled?)", err)
	}
	if res.Hits == nil {
		t.Errorf("expected Hits != nil; got: nil")
	}

	aggs := res.Aggregations
	if aggs == nil {
		t.Fatal("expected aggregations != nil; got: nil")
	}

	agg, found := aggs.DateHistogram("sales_per_month")
	if !found {
		t.Fatal("expected sales_per_month aggregation")
	}
	if agg == nil {
		t.Fatal("expected sales_per_month aggregation")
	}
	if got, want := len(agg.Buckets), 2; got != want {
		t.Fatalf("expected %d buckets; got: %d", want, got)
	}

	if got, want := agg.Buckets[0].DocCount, int64(1); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[1].DocCount, int64(2); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
}

func TestAggsIntegrationSerialDiff(t *testing.T) {
	//client := setupTestClientAndCreateIndexAndAddDocs(t, SetTraceLog(log.New(os.Stdout, "", log.LstdFlags)))
	client := setupTestClientAndCreateIndexAndAddDocs(t)

	esversion, err := client.ElasticsearchVersion(DefaultURL)
	if err != nil {
		t.Fatal(err)
	}

	if esversion < "2.0" {
		t.Skipf("Elasticsearch %s does not have pipeline aggregations.", esversion)
		return
	}

	// Match all should return all documents
	builder := client.Search().
		Index(testIndexName).
		Type("order").
		Query(NewMatchAllQuery()).
		Pretty(true)
	h := NewDateHistogramAggregation().Field("time").Interval("month")
	h = h.SubAggregation("sales", NewSumAggregation().Field("price"))
	h = h.SubAggregation("the_diff", NewSerialDiffAggregation().BucketsPath("sales").Lag(1))
	builder = builder.Aggregation("sales_per_month", h)

	res, err := builder.Do()
	if err != nil {
		t.Fatal(err)
	}
	if res.Hits == nil {
		t.Errorf("expected Hits != nil; got: nil")
	}

	aggs := res.Aggregations
	if aggs == nil {
		t.Fatal("expected aggregations != nil; got: nil")
	}

	agg, found := aggs.DateHistogram("sales_per_month")
	if !found {
		t.Fatal("expected sales_per_month aggregation")
	}
	if agg == nil {
		t.Fatal("expected sales_per_month aggregation")
	}
	if got, want := len(agg.Buckets), 6; got != want {
		t.Fatalf("expected %d buckets; got: %d", want, got)
	}

	if got, want := agg.Buckets[0].DocCount, int64(1); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[1].DocCount, int64(0); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[2].DocCount, int64(1); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[3].DocCount, int64(3); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[4].DocCount, int64(1); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}
	if got, want := agg.Buckets[5].DocCount, int64(2); got != want {
		t.Fatalf("expected DocCount=%d; got: %d", want, got)
	}

	d, found := agg.Buckets[0].SerialDiff("the_diff")
	if found {
		t.Fatal("expected no the_diff aggregation")
	}
	if d != nil {
		t.Fatal("expected no the_diff aggregation")
	}

	d, found = agg.Buckets[1].SerialDiff("the_diff")
	if found {
		t.Fatal("expected no the_diff aggregation")
	}
	if d != nil {
		t.Fatal("expected no the_diff aggregation")
	}

	d, found = agg.Buckets[2].SerialDiff("the_diff")
	if found {
		t.Fatal("expected no the_diff aggregation")
	}
	if d != nil {
		t.Fatal("expected no the_diff aggregation")
	}

	d, found = agg.Buckets[3].SerialDiff("the_diff")
	if !found {
		t.Fatal("expected the_diff aggregation")
	}
	if d == nil {
		t.Fatal("expected the_diff aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected the_diff value != nil")
	}
	if got, want := *d.Value, float64(2348.0); got != want {
		t.Fatalf("expected the_diff.value=%v; got: %v", want, got)
	}

	d, found = agg.Buckets[4].SerialDiff("the_diff")
	if !found {
		t.Fatal("expected the_diff aggregation")
	}
	if d == nil {
		t.Fatal("expected the_diff aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected the_diff value != nil")
	}
	if got, want := *d.Value, float64(-1658.0); got != want {
		t.Fatalf("expected the_diff.value=%v; got: %v", want, got)
	}

	d, found = agg.Buckets[5].SerialDiff("the_diff")
	if !found {
		t.Fatal("expected the_diff aggregation")
	}
	if d == nil {
		t.Fatal("expected the_diff aggregation")
	}
	if d.Value == nil {
		t.Fatal("expected the_diff value != nil")
	}
	if got, want := *d.Value, float64(-722.0); got != want {
		t.Fatalf("expected the_diff.value=%v; got: %v", want, got)
	}
}
