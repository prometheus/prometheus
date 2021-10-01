# Usage

TSDB can be - and is - used by other applications such as [Cortex](https://cortexmetrics.io/) and Thanos (TODO confirm this).
This directory contains documentation for any developers who wish to work on or with TSDB.

Tip: `tsdb/db_test.go` demonstrates various usages of the TSDB library.

## Instantiating a database.

Callers should use [`tsdb.Open`](https://pkg.go.dev/github.com/prometheus/prometheus/tsdb#Open) to open a TSDB.
(the directory may be new or pre-existing).
This returns a [`tsdb.*DB`](https://pkg.go.dev/github.com/prometheus/prometheus/tsdb#DB) which is the actual database.

a `DB` has the following main components:

* Compactor: a leveled compactor. Note: there is currently one compactor implementation. It runs automatically.
* [`Head`](https://pkg.go.dev/github.com/prometheus/prometheus/tsdb#DB.Head)

The `Head` is responsible for a lot.  Here are some of its main components:

* [wal](https://pkg.go.dev/github.com/prometheus/prometheus/tsdb/wal#WAL) (Write Ahead Log)
* [`stripeSeries`](https://github.com/prometheus/prometheus/blob/1270b87970baeb926fcce64552db5c744ffaf83f/tsdb/head.go#L1279):
  this holds all the active series by linking to [`memSeries`](https://github.com/prometheus/prometheus/blob/1270b87970baeb926fcce64552db5c744ffaf83f/tsdb/head.go#L1449)
  by an ID (aka "ref") and by labels hash.
* postings list (reverse index): For any label-value pair, holds all the corresponding series refs. Used for queries.
* tombstones


## Adding data

Use [`db.Appender()`](https://pkg.go.dev/github.com/prometheus/prometheus/tsdb#DB.Appender) to obtain an "appender".
The [golang docs](https://pkg.go.dev/github.com/prometheus/prometheus@v1.8.2-0.20211003130516-1270b87970ba/storage#Appender) speak mostly for themselves.

Remember:

* Use Commit() to update the WAL.
* create a new appender each time you commit.

You can use multiple appenders concurrently (TODO: use case? trade-offs ?)

Append may reject data due to these conditions:

1) `timestamp < minValidTime` where `minValidTime` is the highest of:
  * the maxTime of the last block (i.e. the last truncation time of Head) - updated via [`Head.Truncate()`](https://pkg.go.dev/github.com/prometheus/prometheus/tsdb#Head.Truncate) and [`DB.CompactHead()`](https://github.com/prometheus/prometheus/blob/1270b87970baeb926fcce64552db5c744ffaf83f/tsdb/db.go#L968)
  * `tsdb.min-block-duration/2` older than the max time in the Head chunk. Note that while technically `storage.tsdb.min-block-duration` is configurable, it's a hidden option and changing it is discouraged.  So We can assume this value to be 2h.
  Breaching this condition results in "out of bounds" errors.  
  The first condition assures the block that will be generated doesn't overlap with the previous one (which simplifies querying)  
  The second condition assures the sample won't go into the so called "compaction window", that is the section of the data that might be in process of being saved into a persistent chunk on disk.  (because that logic runs concurrently with ingestion without a lock)
2) The labels don't validate. (if the set is empty or contains duplicate label names)
3) If the sample, for the respective serie (based on all the labels) is out of order or has a different value for the last (highest) timestamp seen. (results in storage.ErrOutOfOrderSample or storage.ErrDuplicateSampleForTimestamp respectively)

Commit() may also refuse data that is out of order with respect to samples that were added via a different appender.

## Querying data

Use [`db.Querier()`](https://pkg.go.dev/github.com/prometheus/prometheus/tsdb#DB.Querier) to obtain a "querier".
The [golang docs](https://pkg.go.dev/github.com/prometheus/prometheus@v1.8.2-0.20211003130516-1270b87970ba/storage#Querier) speak mostly for themselves.

Remember:

* A querier can only see data that was committed when it was created. This limits the lifetime of a querier.
* A querier should be closed when you're done with it.
* TODO: what is the purpose of mint/maxt ? Prevent from reading beyond predefined limits?

Example:

```
querier, err := db.Querier(context.TODO(), 0, math.MaxInt64)
ss := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))

for ss.Next() {
	series := ss.At()
	fmt.Println("series:", series.Labels().String())

	it := series.Iterator()
	for it.Next() {
		t, v := it.At()
		samples = fmt.Println("sample", t,v)
	}

	fmt.Println("it.Err():", it.Err())
}
fmt.Println("ss.Err():", ss.Err())
ws := ss.Warnings()
if len(ws) > 0 {
	fmt.Println("warnings:", ws)
}
err := querier.Close()
```
