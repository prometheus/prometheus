package storage

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrDuplicateSampleForTimestamp(t *testing.T) {
	// All errDuplicateSampleForTimestamp are ErrDuplicateSampleForTimestamp
	require.True(t, errors.Is(ErrDuplicateSampleForTimestamp, errDuplicateSampleForTimestamp{}))

	// Same type only is if it has same properties.
	err1 := NewDuplicateFloatErr(1_000, 10, 20)
	err2 := NewDuplicateFloatErr(1_001, 30, 40)

	require.True(t, errors.Is(err1, err1))
	require.False(t, errors.Is(err1, err2))

	// Also works when err is wrapped.
	require.True(t, errors.Is(fmt.Errorf("failed: %w", err1), err1))
	require.False(t, errors.Is(fmt.Errorf("failed: %w", err1), err2))
}
