package codec

import (
	"testing"

	clientmodel "github.com/prometheus/client_golang/model"
)

func newCodableFingerprint(fp int64) *CodableFingerprint {
	cfp := CodableFingerprint(fp)
	return &cfp
}

func newCodableLabelName(ln string) *CodableLabelName {
	cln := CodableLabelName(ln)
	return &cln
}

func TestCodec(t *testing.T) {
	scenarios := []struct {
		in    codable
		out   codable
		equal func(in, out codable) bool
	}{
		{
			in: &CodableMetric{
				"label_1": "value_2",
				"label_2": "value_2",
				"label_3": "value_3",
			},
			out: &CodableMetric{},
			equal: func(in, out codable) bool {
				m1 := (*clientmodel.Metric)(in.(*CodableMetric))
				m2 := (*clientmodel.Metric)(out.(*CodableMetric))
				return m1.Equal(*m2)
			},
		}, {
			in:  newCodableFingerprint(12345),
			out: newCodableFingerprint(0),
			equal: func(in, out codable) bool {
				return *in.(*CodableFingerprint) == *out.(*CodableFingerprint)
			},
		}, {
			in:  &CodableFingerprints{1, 2, 56, 1234},
			out: &CodableFingerprints{},
			equal: func(in, out codable) bool {
				fps1 := *in.(*CodableFingerprints)
				fps2 := *out.(*CodableFingerprints)
				if len(fps1) != len(fps2) {
					return false
				}
				for i := range fps1 {
					if fps1[i] != fps2[i] {
						return false
					}
				}
				return true
			},
		}, {
			in: &CodableLabelPair{
				Name:  "label_name",
				Value: "label_value",
			},
			out: &CodableLabelPair{},
			equal: func(in, out codable) bool {
				lp1 := *in.(*CodableLabelPair)
				lp2 := *out.(*CodableLabelPair)
				return lp1 == lp2
			},
		}, {
			in:  newCodableLabelName("label_name"),
			out: newCodableLabelName(""),
			equal: func(in, out codable) bool {
				ln1 := *in.(*CodableLabelName)
				ln2 := *out.(*CodableLabelName)
				return ln1 == ln2
			},
		}, {
			in:  &CodableLabelValues{"value_1", "value_2", "value_3"},
			out: &CodableLabelValues{},
			equal: func(in, out codable) bool {
				lvs1 := *in.(*CodableLabelValues)
				lvs2 := *out.(*CodableLabelValues)
				if len(lvs1) != len(lvs2) {
					return false
				}
				for i := range lvs1 {
					if lvs1[i] != lvs2[i] {
						return false
					}
				}
				return true
			},
		}, {
			in:  &CodableTimeRange{42, 2001},
			out: &CodableTimeRange{},
			equal: func(in, out codable) bool {
				ln1 := *in.(*CodableTimeRange)
				ln2 := *out.(*CodableTimeRange)
				return ln1 == ln2
			},
		},
	}

	for i, s := range scenarios {
		encoded, err := s.in.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		if err := s.out.UnmarshalBinary(encoded); err != nil {
			t.Fatal(err)
		}
		if !s.equal(s.in, s.out) {
			t.Errorf("%d. Got: %v; want %v; encoded bytes are: %v", i, s.out, s.in, encoded)
		}
	}
}
