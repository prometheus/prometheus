package importer

import (
	"testing"
)

func TestCSVParser(t *testing.T) {
	for _, tcase := range []struct {
		name  string
		input string
	}{
		{
			name: "empty",
		},
		{
			name:  "just header",
			input: `metric,label,value,timestamp`,
		},
	} {
		t.Run("", func(t *testing.T) {

		})
	}
}
