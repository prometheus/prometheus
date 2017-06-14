package rulefmt

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFileSuccess(t *testing.T) {
	if _, errs := ParseFile("testdata/test.yaml"); len(errs) > 0 {
		t.Errorf("unexpected errors parsing file")
		for _, err := range errs {
			t.Error(err)
		}
	}
}

func TestParseFileFailure(t *testing.T) {
	table := []struct {
		filename string
		errMsg   string
	}{
		{
			filename: "duplicate_grp.bad.yaml",
			errMsg:   "groupname: \"yolo\" is repeated in the same file",
		},
		{
			filename: "noversion.bad.yaml",
			errMsg:   "invalid rule group version 0",
		},
		{
			filename: "bad_expr.bad.yaml",
			errMsg:   "parse error",
		},
		{
			filename: "record_and_alert.bad.yaml",
			errMsg:   "only one of 'record' and 'alert' must be set",
		},
		{
			filename: "no_rec_alert.bad.yaml",
			errMsg:   "one of 'record' or 'alert' must be set",
		},
		{
			filename: "noexpr.bad.yaml",
			errMsg:   "field 'expr' must be set in rule",
		},
	}

	for _, c := range table {
		_, errs := ParseFile(filepath.Join("testdata", c.filename))
		if errs == nil {
			t.Errorf("Expected error parsing %s but got none", c.filename)
			continue
		}
		if !strings.Contains(errs[0].Error(), c.errMsg) {
			t.Errorf("Expected error for %s to contain %q but got: %s", c.filename, c.errMsg, errs)
		}
	}

}
