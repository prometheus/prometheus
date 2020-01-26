package logfmt

import (
	"bufio"
	"bytes"
	"testing"

	kr "github.com/kr/logfmt"
)

func BenchmarkDecodeKeyval(b *testing.B) {
	const rows = 10000
	data := []byte{}
	for i := 0; i < rows; i++ {
		data = append(data, "a=1 b=\"bar\" ƒ=2h3s r=\"esc\\tmore stuff\" d x=sf   \n"...)
	}

	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var (
			dec = NewDecoder(bytes.NewReader(data))
			j   = 0
		)
		for dec.ScanRecord() {
			for dec.ScanKeyval() {
			}
			j++
		}
		if err := dec.Err(); err != nil {
			b.Errorf("got %v, want %v", err, nil)
		}
		if j != rows {
			b.Errorf("got %v, want %v", j, rows)
		}
	}
}

func BenchmarkKRDecode(b *testing.B) {
	const rows = 10000
	data := []byte{}
	for i := 0; i < rows; i++ {
		data = append(data, "a=1 b=\"bar\" ƒ=2h3s r=\"esc\\tmore stuff\" d x=sf   \n"...)
	}

	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var (
			s   = bufio.NewScanner(bytes.NewReader(data))
			err error
			j   = 0
			dh  discardHandler
		)
		for err == nil && s.Scan() {
			err = kr.Unmarshal(s.Bytes(), &dh)
			j++
		}
		if err == nil {
			err = s.Err()
		}
		if err != nil {
			b.Errorf("got %v, want %v", err, nil)
		}
		if j != rows {
			b.Errorf("got %v, want %v", j, rows)
		}
	}
}

type discardHandler struct{}

func (discardHandler) HandleLogfmt(key, val []byte) error {
	return nil
}
