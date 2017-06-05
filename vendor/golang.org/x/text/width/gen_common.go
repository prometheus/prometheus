// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

package main

// This code is shared between the main code generator and the test code.

import (
	"flag"
	"log"
	"strconv"
	"strings"

	"golang.org/x/text/internal/gen"
	"golang.org/x/text/internal/ucd"
)

var (
	outputFile = flag.String("out", "tables.go", "output file")
)

var typeMap = map[string]elem{
	"A":  tagAmbiguous,
	"N":  tagNeutral,
	"Na": tagNarrow,
	"W":  tagWide,
	"F":  tagFullwidth,
	"H":  tagHalfwidth,
}

// getWidthData calls f for every entry for which it is defined.
//
// f may be called multiple times for the same rune. The last call to f is the
// correct value. f is not called for all runes. The default tag type is
// Neutral.
func getWidthData(f func(r rune, tag elem, alt rune)) {
	// Set the default values for Unified Ideographs. In line with Annex 11,
	// we encode full ranges instead of the defined runes in Unified_Ideograph.
	for _, b := range []struct{ lo, hi rune }{
		{0x4E00, 0x9FFF},   // the CJK Unified Ideographs block,
		{0x3400, 0x4DBF},   // the CJK Unified Ideographs Externsion A block,
		{0xF900, 0xFAFF},   // the CJK Compatibility Ideographs block,
		{0x20000, 0x2FFFF}, // the Supplementary Ideographic Plane,
		{0x30000, 0x3FFFF}, // the Tertiary Ideographic Plane,
	} {
		for r := b.lo; r <= b.hi; r++ {
			f(r, tagWide, 0)
		}
	}

	inverse := map[rune]rune{}
	maps := map[string]bool{
		"<wide>":   true,
		"<narrow>": true,
	}

	// We cannot reuse package norm's decomposition, as we need an unexpanded
	// decomposition. We make use of the opportunity to verify that the
	// decomposition type is as expected.
	ucd.Parse(gen.OpenUCDFile("UnicodeData.txt"), func(p *ucd.Parser) {
		r := p.Rune(0)
		s := strings.SplitN(p.String(ucd.DecompMapping), " ", 2)
		if !maps[s[0]] {
			return
		}
		x, err := strconv.ParseUint(s[1], 16, 32)
		if err != nil {
			log.Fatalf("Error parsing rune %q", s[1])
		}
		if inverse[r] != 0 || inverse[rune(x)] != 0 {
			log.Fatalf("Circular dependency in mapping between %U and %U", r, x)
		}
		inverse[r] = rune(x)
		inverse[rune(x)] = r
	})

	// <rune range>;<type>
	ucd.Parse(gen.OpenUCDFile("EastAsianWidth.txt"), func(p *ucd.Parser) {
		tag, ok := typeMap[p.String(1)]
		if !ok {
			log.Fatalf("Unknown width type %q", p.String(1))
		}
		r := p.Rune(0)
		alt, ok := inverse[r]
		if tag == tagFullwidth || tag == tagHalfwidth && r != wonSign {
			tag |= tagNeedsFold
			if !ok {
				log.Fatalf("Narrow or wide rune %U has no decomposition", r)
			}
		}
		f(r, tag, alt)
	})
}
