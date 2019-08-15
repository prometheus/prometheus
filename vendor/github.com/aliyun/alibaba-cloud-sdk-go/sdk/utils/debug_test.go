package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMain(t *testing.T) {
	debug := Init("sdk")
	debug("%s", "testing")
}

func TestMain_Matched(t *testing.T) {
	originLookGetEnv := hookGetEnv
	originhookPrint := hookPrint
	defer func() {
		hookGetEnv = originLookGetEnv
		hookPrint = originhookPrint
	}()
	hookGetEnv = func() string {
		return "sdk"
	}
	output := ""
	hookPrint = func(input string) {
		output = input
		originhookPrint(input)
	}
	debug := Init("sdk")
	debug("%s", "testing")
	assert.Equal(t, "testing", output)
}

func TestMain_UnMatched(t *testing.T) {
	originLookGetEnv := hookGetEnv
	originhookPrint := hookPrint
	defer func() {
		hookGetEnv = originLookGetEnv
		hookPrint = originhookPrint
	}()
	hookGetEnv = func() string {
		return "non-sdk"
	}
	output := ""
	hookPrint = func(input string) {
		output = input
		originhookPrint(input)
	}
	debug := Init("sdk")
	debug("%s", "testing")
	assert.Equal(t, "", output)
}
