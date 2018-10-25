package lint

import (
	"bufio"
	"bytes"
	"io"
)

var (
	prefix = []byte("// Code generated ")
	suffix = []byte(" DO NOT EDIT.")
	nl     = []byte("\n")
	crnl   = []byte("\r\n")
)

func isGenerated(r io.Reader) bool {
	br := bufio.NewReader(r)
	for {
		s, err := br.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return false
		}
		s = bytes.TrimSuffix(s, crnl)
		s = bytes.TrimSuffix(s, nl)
		if bytes.HasPrefix(s, prefix) && bytes.HasSuffix(s, suffix) {
			return true
		}
		if err == io.EOF {
			break
		}
	}
	return false
}
