package promql

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/prometheus/promql/parser"
)

type StubNode struct {
	start, end int
}

func (s StubNode) String() string {
	return fmt.Sprintf("[%d,%d]", s.start, s.end)
}

func (s StubNode) PositionRange() parser.PositionRange {
	return parser.PositionRange{
		Start: parser.Pos(s.start),
		End:   parser.Pos(s.end),
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
					Prefix: []parser.PositionRange{
						parser.PositionRange{parser.Pos(0), parser.Pos(1)},
						parser.PositionRange{parser.Pos(1), parser.Pos(3)},
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
