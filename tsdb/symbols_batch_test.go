package tsdb

import (
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSymbolsBatchAndIteration(t *testing.T) {
	dir := t.TempDir()

	b := newSymbolsBatcher(100, dir)

	allWords := map[string]struct{}{}

	for i := 0; i < 10; i++ {
		require.NoError(t, b.addSymbol(""))
		allWords[""] = struct{}{}

		for j := 0; j < 123; j++ {
			w := fmt.Sprintf("word_%d_%d", i%3, j)

			require.NoError(t, b.addSymbol(w))

			allWords[w] = struct{}{}
		}
	}

	require.NoError(t, b.flushSymbols(true))

	it, err := newSymbolsIterator(b.symbolFiles())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, it.Close())
	})

	first := true
	var w, prev string
	for w, err = it.NextSymbol(); err == nil; w, err = it.NextSymbol() {
		if !first {
			require.True(t, w != "")
			require.True(t, prev < w)
		}

		first = false

		_, known := allWords[w]
		require.True(t, known)
		delete(allWords, w)
		prev = w
	}
	require.Equal(t, io.EOF, err)
	require.Equal(t, 0, len(allWords))
}
