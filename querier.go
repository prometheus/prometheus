package tsdb

// Matcher matches a string.
type Matcher interface {
	// Match returns true if the matcher applies to the string value.
	Match(v string) bool
}

// Querier provides querying access over time series data of a fixed
// time range.
type Querier interface {
	// Range returns the timestamp range of the Querier.
	Range() (start, end int64)

	// Iterator returns an interator over the inverted index that
	// matches the key label by the constraints of Matcher.
	Iterator(key string, m Matcher) Iterator

	// Labels resolves a label reference into a set of labels.
	Labels(ref LabelRefs) (Labels, error)

	// Series returns series provided in the index iterator.
	Series(Iterator) []Series

	// LabelValues returns all potential values for a label name.
	LabelValues(string) []string
	// LabelValuesFor returns all potential values for a label name.
	// under the constraint of another label.
	LabelValuesFor(string, Label) []string

	// Close releases the resources of the Querier.
	Close() error
}

// Series represents a single time series.
type Series interface {
	Labels() Labels
	// Iterator returns a new iterator of the data of the series.
	Iterator() SeriesIterator
}

// SeriesIterator iterates over the data of a time series.
type SeriesIterator interface {
	// Seek advances the iterator forward to the given timestamp.
	// If there's no value exactly at ts, it advances to the last value
	// before tt.
	Seek(t int64) bool
	// Values returns the current timestamp/value pair.
	Values() (t int64, v float64)
	// Next advances the iterator by one.
	Next() bool
	// Err returns the current error.
	Err() error
}
