package klog

import (
	"bytes"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
)

func TestSetLogger(t *testing.T) {
	l := log.NewNopLogger()
	SetLogger(l)

	assert.Equal(t, l, logger)
}

func TestV(t *testing.T) {
	assert.True(t, bool(V(0)))
}

func TestInfo(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(log.NewJSONLogger(&buf))

	testcases := []struct {
		Name     string
		Args     []interface{}
		Expected string
	}{{
		Name:     "String",
		Args:     []interface{}{"foo"},
		Expected: `{"msg":"foo", "func":"Info", "level":"debug"}`,
	}, {
		Name:     "Int",
		Args:     []interface{}{42},
		Expected: `{"msg":"42", "func":"Info", "level":"debug"}`,
	}, {
		Name:     "Bool",
		Args:     []interface{}{true},
		Expected: `{"msg":"true", "func":"Info", "level":"debug"}`,
	}, {
		Name:     "StringInt",
		Args:     []interface{}{"foo", 42},
		Expected: `{"msg":"foo42", "func":"Info", "level":"debug"}`,
	}}

	for _, tc := range testcases {
		t.Run(tc.Name, func(t *testing.T) {
			Info(tc.Args...)
			assert.JSONEq(t, tc.Expected, buf.String())
			buf.Reset()
		})
	}
}
