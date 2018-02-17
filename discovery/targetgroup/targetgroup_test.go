package targetgroup

import (
	"errors"
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestTargetGroupStrictJsonUnmarshal(t *testing.T) {
	tests := []struct {
		json          string
		expectedReply error
	}{
		{
			json: `	{"labels": {},"targets": []}`,
			expectedReply: nil,
		},
		{
			json: `	{"label": {},"targets": []}`,
			expectedReply: errors.New("json: unknown field \"label\""),
		},
		{
			json: `	{"labels": {},"target": []}`,
			expectedReply: errors.New("json: unknown field \"target\""),
		},
	}
	tg := Group{}

	for _, test := range tests {
		actual := tg.UnmarshalJSON([]byte(test.json))
		testutil.Equals(t, test.expectedReply, actual)
	}

}
