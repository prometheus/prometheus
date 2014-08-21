package index

import (
	"testing"

	clientmodel "github.com/prometheus/client_golang/model"
)

func newCodableFingerprint(fp int64) *codableFingerprint {
	cfp := codableFingerprint(fp)
	return &cfp
}

func newCodableLabelName(ln string) *codableLabelName {
	cln := codableLabelName(ln)
	return &cln
}

func TestCodec(t *testing.T) {
	scenarios := []struct {
		in    codable
		out   codable
		equal func(in, out codable) bool
	}{
		{
			in: codableMetric{
				"label_1": "value_2",
				"label_2": "value_2",
				"label_3": "value_3",
			},
			out: codableMetric{},
			equal: func(in, out codable) bool {
				m1 := clientmodel.Metric(in.(codableMetric))
				m2 := clientmodel.Metric(out.(codableMetric))
				return m1.Equal(m2)
			},
		}, {
			in:  newCodableFingerprint(12345),
			out: newCodableFingerprint(0),
			equal: func(in, out codable) bool {
				return *in.(*codableFingerprint) == *out.(*codableFingerprint)
			},
		}, {
			in:  &codableFingerprints{1, 2, 56, 1234},
			out: &codableFingerprints{},
			equal: func(in, out codable) bool {
				fps1 := *in.(*codableFingerprints)
				fps2 := *out.(*codableFingerprints)
				if len(fps1) != len(fps2) {
					return false
				}
				for i, _ := range fps1 {
					if fps1[i] != fps2[i] {
						return false
					}
				}
				return true
			},
		}, {
			in: &codableLabelPair{
				Name:  "label_name",
				Value: "label_value",
			},
			out: &codableLabelPair{},
			equal: func(in, out codable) bool {
				lp1 := *in.(*codableLabelPair)
				lp2 := *out.(*codableLabelPair)
				return lp1 == lp2
			},
		}, {
			in:  newCodableLabelName("label_name"),
			out: newCodableLabelName(""),
			equal: func(in, out codable) bool {
				ln1 := *in.(*codableLabelName)
				ln2 := *out.(*codableLabelName)
				return ln1 == ln2
			},
		}, {
			in:  &codableLabelValues{"value_1", "value_2", "value_3"},
			out: &codableLabelValues{},
			equal: func(in, out codable) bool {
				lvs1 := *in.(*codableLabelValues)
				lvs2 := *out.(*codableLabelValues)
				if len(lvs1) != len(lvs2) {
					return false
				}
				for i, _ := range lvs1 {
					if lvs1[i] != lvs2[i] {
						return false
					}
				}
				return true
			},
		}, {
			in:  &codableMembership{},
			out: &codableMembership{},
			equal: func(in, out codable) bool {
				// We don't care about the membership value. Just test if the
				// encoding/decoding works at all.
				return true
			},
		},
	}

	for i, s := range scenarios {
		encoded := s.in.encode()
		s.out.decode(encoded)
		if !s.equal(s.in, s.out) {
			t.Fatalf("%d. Got: %v; want %v; encoded bytes are: %v", i, s.out, s.in, encoded)
		}
	}
}
