package rulefmt

import "testing"

func TestParseFileSuccess(t *testing.T) {
	if _, errs := ParseFile("testdata/test.yaml"); len(errs) > 0 {
		t.Errorf("unexpected errors parsing file")
		for _, err := range errs {
			t.Error(err)
		}
	}
}
