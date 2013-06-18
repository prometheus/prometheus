package metric

import (
	"time"

	clientmodel "github.com/prometheus/client_golang/model"
)

func (s SamplePair) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("{\"Value\": \"%f\", \"Timestamp\": %d}", s.Value, s.Timestamp.Unix())), nil
}

type SamplePair struct {
	Value     clientmodel.SampleValue
	Timestamp time.Time
}

func (s SamplePair) Equal(o SamplePair) bool {
	return s.Value.Equal(o.Value) && s.Timestamp.Equal(o.Timestamp)
}

func (s SamplePair) ToDTO() (out *dto.SampleValueSeries_Value) {
	out = &dto.SampleValueSeries_Value{
		Timestamp: proto.Int64(s.Timestamp.Unix()),
		Value:     s.Value.ToDTO(),
	}

	return
}

func (s SamplePair) String() string {
	return fmt.Sprintf("SamplePair at %s of %s", s.Timestamp, s.Value)
}

type Values []SamplePair

func (v Values) Len() int {
	return len(v)
}

func (v Values) Less(i, j int) bool {
	return v[i].Timestamp.Before(v[j].Timestamp)
}

func (v Values) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

// FirstTimeAfter indicates whether the first sample of a set is after a given
// timestamp.
func (v Values) FirstTimeAfter(t time.Time) bool {
	return v[0].Timestamp.After(t)
}

// LastTimeBefore indicates whether the last sample of a set is before a given
// timestamp.
func (v Values) LastTimeBefore(t time.Time) bool {
	return v[len(v)-1].Timestamp.Before(t)
}

// InsideInterval indicates whether a given range of sorted values could contain
// a value for a given time.
func (v Values) InsideInterval(t time.Time) bool {
	switch {
	case v.Len() == 0:
		return false
	case t.Before(v[0].Timestamp):
		return false
	case !v[v.Len()-1].Timestamp.Before(t):
		return false
	default:
		return true
	}
}

// TruncateBefore returns a subslice of the original such that extraneous
// samples in the collection that occur before the provided time are
// dropped.  The original slice is not mutated
func (v Values) TruncateBefore(t time.Time) Values {
	index := sort.Search(len(v), func(i int) bool {
		timestamp := v[i].Timestamp

		return !timestamp.Before(t)
	})

	return v[index:]
}

func (v Values) ToDTO() (out *dto.SampleValueSeries) {
	out = &dto.SampleValueSeries{}

	for _, value := range v {
		out.Value = append(out.Value, value.ToDTO())
	}

	return
}

func (v Values) ToSampleKey(f *Fingerprint) SampleKey {
	return SampleKey{
		Fingerprint:    f,
		FirstTimestamp: v[0].Timestamp,
		LastTimestamp:  v[len(v)-1].Timestamp,
		SampleCount:    uint32(len(v)),
	}
}

func (v Values) String() string {
	buffer := bytes.Buffer{}

	fmt.Fprintf(&buffer, "[")
	for i, value := range v {
		fmt.Fprintf(&buffer, "%d. %s", i, value)
		if i != len(v)-1 {
			fmt.Fprintf(&buffer, "\n")
		}
	}
	fmt.Fprintf(&buffer, "]")

	return buffer.String()
}

func NewValuesFromDTO(d *dto.SampleValueSeries) Values {
	v := make(Values, 0, len(d.Value))

	for _, value := range d.Value {
		v = append(v, SamplePair{
			Timestamp: time.Unix(value.GetTimestamp(), 0).UTC(),
			Value:     SampleValue(*value.Value),
		})
	}

	return v
}

type SampleSet struct {
	Metric Metric
	Values Values
}

type Interval struct {
	OldestInclusive time.Time
	NewestInclusive time.Time
}
