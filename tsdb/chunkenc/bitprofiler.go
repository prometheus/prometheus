// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chunkenc

import (
	"fmt"
	"strings"
)

type piece[T any] struct {
	value T
	bits  int64
	what  string
}
type bitProfiler[T any] struct {
	samples []piece[T]
}

func (p *bitProfiler[T]) Reset() {
	p.samples = p.samples[:0]
}

func (p *bitProfiler[T]) Write(b *bstream, value T, what string, do func()) {
	bits := b.lenBits()
	do()
	p.samples = append(p.samples, piece[T]{bits: int64(b.lenBits() - bits), what: what, value: value})
}

func (p *bitProfiler[T]) Add(bits int64, value T, what string) {
	p.samples = append(p.samples, piece[T]{bits: bits, what: what, value: value})
}

func (p *bitProfiler[T]) PrintBitSizes(trim int) string {
	if len(p.samples) == 0 {
		return "total(0b)"
	}
	if trim > len(p.samples) {
		trim = len(p.samples)
	}

	var total int64
	for _, p := range p.samples {
		total += p.bits
	}
	var cells []string
	for _, p := range p.samples[:trim] {
		cellStr := fmt.Sprintf("%s(%s)", p.what, writeBits(p.bits))
		cells = append(cells, cellStr)
	}

	var sb strings.Builder
	sb.WriteString("total(")
	sb.WriteString(writeBits(total))
	sb.WriteString(") |")
	for _, cell := range cells {
		sb.WriteString(" ") // Left padding
		sb.WriteString(cell)
		sb.WriteString(" ") // Right padding
		sb.WriteString("│")
	}
	sb.WriteString("...")

	return sb.String()
}

func (p *bitProfiler[T]) PrintBitSizesAndValues(trim int) string {
	if len(p.samples) == 0 {
		return "total(0b)"
	}
	if trim > len(p.samples) {
		trim = len(p.samples)
	}

	var total int64
	for _, p := range p.samples {
		total += p.bits
	}
	var cells []string
	for _, p := range p.samples[:trim] {
		cellStr := fmt.Sprintf("%s(%s)[%#v]", p.what, writeBits(p.bits), p.value)
		cells = append(cells, cellStr)
	}

	var sb strings.Builder
	sb.WriteString("total(")
	sb.WriteString(writeBits(total))
	sb.WriteString(") |")
	for _, cell := range cells {
		sb.WriteString(" ") // Left padding
		sb.WriteString(cell)
		sb.WriteString(" ") // Right padding
		sb.WriteString("│")
	}
	sb.WriteString("...")

	return sb.String()
}

func writeBits(bits int64) string {
	var bytesStr, bitsStr string
	if bits/8 > 0 {
		bytesStr = fmt.Sprintf("%dB ", bits/8)
	}
	if bits%8 > 0 {
		bitsStr = fmt.Sprintf("%db", bits%8)
	}
	return fmt.Sprintf("%s%s", bytesStr, bitsStr)
}

func (p *bitProfiler[T]) Diff(other *bitProfiler[T]) *bitProfiler[T] {
	ret := &bitProfiler[T]{}
	for i, p := range p.samples {
		ret.samples = append(ret.samples, piece[T]{bits: p.bits - other.samples[i].bits, what: other.samples[i].what})
	}
	return ret
}

type dodSample struct {
	t      int64
	tDelta uint64
	dod    int64
}
