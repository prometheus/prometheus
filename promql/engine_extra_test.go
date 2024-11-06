package promql

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
)

type StubNode struct {
	start, end int
}

func (s StubNode) String() string {
	return fmt.Sprintf("[%d,%d]", s.start, s.end)
}

func (s StubNode) Pretty(t int) string {
	return fmt.Sprintf("%d", t)
}

func (s StubNode) PositionRange() posrange.PositionRange {
	return posrange.PositionRange{
		Start: posrange.Pos(s.start),
		End:   posrange.Pos(s.end),
	}
}

func TestFindPathRange(t *testing.T) {
	tests := []struct {
		path    []parser.Node
		eRanges []evalRange
		out     time.Duration
	}{
		// Test a case where the evalRange is longer than the path
		{
			path: []parser.Node{StubNode{0, 1}},
			eRanges: []evalRange{
				evalRange{
					Prefix: []posrange.PositionRange{
						{posrange.Pos(0), posrange.Pos(1)},
						{posrange.Pos(1), posrange.Pos(3)},
					},
				},
			},
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			out := findPathRange(test.path, test.eRanges)
			if !reflect.DeepEqual(out, test.out) {
				t.Fatalf("Mismatch in test output expected=%#v actual=%#v", test.out, out)
			}
		})
	}
}
