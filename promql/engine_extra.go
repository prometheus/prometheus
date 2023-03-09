package promql

import (
	"time"

	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
)

func findPathRange(path []parser.Node, eRanges []evalRange) time.Duration {
	var (
		evalRange time.Duration
		depth     int
	)
	for _, r := range eRanges {
		// Check if we are a child
		child := true
		for i, p := range r.Prefix {
			if p != path[i].PositionRange() {
				child = false
				break
			}
		}
		if child && len(r.Prefix) > depth {
			evalRange = r.Range
			depth = len(r.Prefix)
		}
	}

	return evalRange
}

// evalRange summarizes a defined evalRange (from a MatrixSelector) within the asg
type evalRange struct {
	Prefix []posrange.PositionRange
	Range  time.Duration
}
