//go:build stringlabels

// Split out function which needs to be coded differently for stringlabels case.

package tsdb

import "strings"

func (sw *symbolsBatcher) addSymbol(sym string) error {
	if _, found := sw.buffer[sym]; !found {
		sym = strings.Clone(sym) // So we don't retain reference to the entire labels block.
		sw.buffer[sym] = struct{}{}
	}
	return sw.flushSymbols(false)
}
