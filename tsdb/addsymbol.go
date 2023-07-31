//go:build !stringlabels

// Split out function which needs to be coded differently for stringlabels case.

package tsdb

func (sw *symbolsBatcher) addSymbol(sym string) error {
	sw.buffer[sym] = struct{}{}
	return sw.flushSymbols(false)
}
