package dns

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateRangeGuard(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "dns")
	if err != nil {
		t.Fatalf("could not create tmpdir for test: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	for i := 0; i <= 1; i++ {
		path := filepath.Join(tmpdir, fmt.Sprintf("%04d.conf", i))
		data := []byte(fmt.Sprintf("dhcp-%04d A 10.0.0.%d", i, i))

		if err := ioutil.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("could not create tmpfile for test: %v", err)
		}
	}

	var tests = [...]struct {
		zone string
		fail bool
	}{
		{`@ IN SOA ns.test. hostmaster.test. ( 1 8h 2h 7d 1d )
$GENERATE 0-1 dhcp-${0,4,d} A 10.0.0.$
`, false},
		{`@ IN SOA ns.test. hostmaster.test. ( 1 8h 2h 7d 1d )
$GENERATE 0-1 dhcp-${0,0,x} A 10.0.0.$
`, false},
		{`@ IN SOA ns.test. hostmaster.test. ( 1 8h 2h 7d 1d )
$GENERATE 128-129 dhcp-${-128,4,d} A 10.0.0.$
`, false},
		{`@ IN SOA ns.test. hostmaster.test. ( 1 8h 2h 7d 1d )
$GENERATE 128-129 dhcp-${-129,4,d} A 10.0.0.$
`, true},
		{`@ IN SOA ns.test. hostmaster.test. ( 1 8h 2h 7d 1d )
$GENERATE 0-2 dhcp-${2147483647,4,d} A 10.0.0.$
`, true},
		{`@ IN SOA ns.test. hostmaster.test. ( 1 8h 2h 7d 1d )
$GENERATE 0-1 dhcp-${2147483646,4,d} A 10.0.0.$
`, false},
		{`@ IN SOA ns.test. hostmaster.test. ( 1 8h 2h 7d 1d )
$GENERATE 0-1/step dhcp-${0,4,d} A 10.0.0.$
`, true},
		{`@ IN SOA ns.test. hostmaster.test. ( 1 8h 2h 7d 1d )
$GENERATE 0-1/ dhcp-${0,4,d} A 10.0.0.$
`, true},
		{`@ IN SOA ns.test. hostmaster.test. ( 1 8h 2h 7d 1d )
$GENERATE 0-10/2 dhcp-${0,4,d} A 10.0.0.$
`, false},
		{`@ IN SOA ns.test. hostmaster.test. ( 1 8h 2h 7d 1d )
$GENERATE 0-1/0 dhcp-${0,4,d} A 10.0.0.$
`, true},
		{`@ IN SOA ns.test. hostmaster.test. ( 1 8h 2h 7d 1d )
$GENERATE 0-1 $$INCLUDE ` + tmpdir + string(filepath.Separator) + `${0,4,d}.conf
`, false},
	}
Outer:
	for i := range tests {
		for tok := range ParseZone(strings.NewReader(tests[i].zone), "test.", "test") {
			if tok.Error != nil {
				if !tests[i].fail {
					t.Errorf("expected \n\n%s\nto be parsed, but got %v", tests[i].zone, tok.Error)
				}
				continue Outer
			}
		}
		if tests[i].fail {
			t.Errorf("expected \n\n%s\nto fail, but got no error", tests[i].zone)
		}
	}
}

func TestGenerateIncludeDepth(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "dns")
	if err != nil {
		t.Fatalf("could not create tmpfile for test: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	zone := `@ IN SOA ns.test. hostmaster.test. ( 1 8h 2h 7d 1d )
$GENERATE 0-1 $$INCLUDE ` + tmpfile.Name() + `
`
	if _, err := tmpfile.WriteString(zone); err != nil {
		t.Fatalf("could not write to tmpfile for test: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatalf("could not close tmpfile for test: %v", err)
	}

	zp := NewZoneParser(strings.NewReader(zone), ".", tmpfile.Name())
	zp.SetIncludeAllowed(true)

	for _, ok := zp.Next(); ok; _, ok = zp.Next() {
	}

	const expected = "too deeply nested $INCLUDE"
	if err := zp.Err(); err == nil || !strings.Contains(err.Error(), expected) {
		t.Errorf("expected error to include %q, got %v", expected, err)
	}
}

func TestGenerateIncludeDisallowed(t *testing.T) {
	const zone = `@ IN SOA ns.test. hostmaster.test. ( 1 8h 2h 7d 1d )
$GENERATE 0-1 $$INCLUDE test.conf
`
	zp := NewZoneParser(strings.NewReader(zone), ".", "")

	for _, ok := zp.Next(); ok; _, ok = zp.Next() {
	}

	const expected = "$INCLUDE directive not allowed"
	if err := zp.Err(); err == nil || !strings.Contains(err.Error(), expected) {
		t.Errorf("expected error to include %q, got %v", expected, err)
	}
}

func TestGenerateSurfacesErrors(t *testing.T) {
	const zone = `@ IN SOA ns.test. hostmaster.test. ( 1 8h 2h 7d 1d )
$GENERATE 0-1 dhcp-${0,4,dd} A 10.0.0.$
`
	zp := NewZoneParser(strings.NewReader(zone), ".", "test")

	for _, ok := zp.Next(); ok; _, ok = zp.Next() {
	}

	const expected = `test: dns: bad base in $GENERATE: "${0,4,dd}" at line: 2:20`
	if err := zp.Err(); err == nil || err.Error() != expected {
		t.Errorf("expected specific error, wanted %q, got %v", expected, err)
	}
}

func TestGenerateSurfacesLexerErrors(t *testing.T) {
	const zone = `@ IN SOA ns.test. hostmaster.test. ( 1 8h 2h 7d 1d )
$GENERATE 0-1 dhcp-${0,4,d} A 10.0.0.$ )
`
	zp := NewZoneParser(strings.NewReader(zone), ".", "test")

	for _, ok := zp.Next(); ok; _, ok = zp.Next() {
	}

	const expected = `test: dns: bad data in $GENERATE directive: "extra closing brace" at line: 2:40`
	if err := zp.Err(); err == nil || err.Error() != expected {
		t.Errorf("expected specific error, wanted %q, got %v", expected, err)
	}
}

func TestGenerateModToPrintf(t *testing.T) {
	tests := []struct {
		mod        string
		wantFmt    string
		wantOffset int
		wantErr    bool
	}{
		{"0,0,d", "%d", 0, false},
		{"0,0", "%d", 0, false},
		{"0", "%d", 0, false},
		{"3,2,d", "%02d", 3, false},
		{"3,2", "%02d", 3, false},
		{"3", "%d", 3, false},
		{"0,0,o", "%o", 0, false},
		{"0,0,x", "%x", 0, false},
		{"0,0,X", "%X", 0, false},
		{"0,0,z", "", 0, true},
		{"0,0,0,d", "", 0, true},
		{"-100,0,d", "%d", -100, false},
	}
	for _, test := range tests {
		gotFmt, gotOffset, errMsg := modToPrintf(test.mod)
		switch {
		case errMsg != "" && !test.wantErr:
			t.Errorf("modToPrintf(%q) - expected empty-error, but got %v", test.mod, errMsg)
		case errMsg == "" && test.wantErr:
			t.Errorf("modToPrintf(%q) - expected error, but got empty-error", test.mod)
		case gotFmt != test.wantFmt:
			t.Errorf("modToPrintf(%q) - expected format %q, but got %q", test.mod, test.wantFmt, gotFmt)
		case gotOffset != test.wantOffset:
			t.Errorf("modToPrintf(%q) - expected offset %d, but got %d", test.mod, test.wantOffset, gotOffset)
		}
	}
}

func BenchmarkGenerate(b *testing.B) {
	const zone = `@ IN SOA ns.test. hostmaster.test. ( 1 8h 2h 7d 1d )
$GENERATE 32-158 dhcp-${-32,4,d} A 10.0.0.$
`

	for n := 0; n < b.N; n++ {
		zp := NewZoneParser(strings.NewReader(zone), ".", "")

		for _, ok := zp.Next(); ok; _, ok = zp.Next() {
		}

		if err := zp.Err(); err != nil {
			b.Fatal(err)
		}
	}
}
