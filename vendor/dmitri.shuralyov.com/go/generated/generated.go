// Package generated provides a function that parses a Go file and reports
// whether it contains a "// Code generated … DO NOT EDIT." line comment.
//
// It implements the specification at https://golang.org/s/generatedcode.
//
// The first priority is correctness (no false negatives, no false positives).
// It must return accurate results even if the input Go source code is not gofmted.
//
// The second priority is performance. The current version uses bufio.Reader and
// ReadBytes. Performance can be optimized further by using lower level I/O
// primitives and allocating less. That can be explored later. A lot of the time
// is spent on reading the entire file without being able to stop early,
// since the specification allows the comment to appear anywhere in the file.
package generated

import (
	"bufio"
	"bytes"
	"io"
	"os"
)

// Parse parses the source code of a single Go source file
// provided via src, and reports whether the file contains
// a "// Code generated ... DO NOT EDIT." line comment
// matching the specification at https://golang.org/s/generatedcode:
//
// 	Generated files are marked by a line of text that matches
// 	the regular expression, in Go syntax:
//
// 		^// Code generated .* DO NOT EDIT\.$
//
// 	The .* means the tool can put whatever folderol it wants in there,
// 	but the comment must be a single line and must start with Code generated
// 	and end with DO NOT EDIT., with a period.
//
// 	The text may appear anywhere in the file.
func Parse(src io.Reader) (hasGeneratedComment bool, err error) {
	br := bufio.NewReader(src)
	for {
		s, err := br.ReadBytes('\n')
		if err == io.EOF {
			return containsComment(s), nil
		} else if err != nil {
			return false, err
		}
		if len(s) >= 2 && s[len(s)-2] == '\r' {
			s = s[:len(s)-2] // Trim "\r\n".
		} else {
			s = s[:len(s)-1] // Trim "\n".
		}
		if containsComment(s) {
			return true, nil
		}
	}
}

// containsComment reports whether a line of Go source code s (without newline character)
// contains the generated comment.
func containsComment(s []byte) bool {
	return len(s) >= len(prefix)+len(suffix) &&
		bytes.HasPrefix(s, prefix) &&
		bytes.HasSuffix(s, suffix)
}

var (
	prefix = []byte("// Code generated ")
	suffix = []byte(" DO NOT EDIT.")
)

// ParseFile opens the file specified by filename and uses Parse to parse it.
// If the source couldn't be read, the error indicates the specific failure.
func ParseFile(filename string) (hasGeneratedComment bool, err error) {
	f, err := os.Open(filename)
	if err != nil {
		return false, err
	}
	defer f.Close()
	return Parse(f)
}
