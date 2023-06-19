package transport_test

import (
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/transport"
)

func TestRefillMsg(t *testing.T) {
	wrm := &transport.RefillMsg{
		Messages: []transport.MessageData{
			{ID: 0, Size: 5, Typemsg: 2},
			{ID: 1, Size: 6, Typemsg: 3},
			{ID: 2, Size: 12, Typemsg: 4},
			{ID: 3, Size: 4, Typemsg: 5},
			{ID: 4294967294, Size: 4294967294, Typemsg: 5},
		},
	}

	b, _ := wrm.MarshalBinary()
	rrm := &transport.RefillMsg{}
	err := rrm.UnmarshalBinary(b)
	require.NoError(t, err)

	require.Equal(t, len(wrm.Messages), len(rrm.Messages))

	for i := range wrm.Messages {
		require.Equal(t, wrm.Messages[i], rrm.Messages[i])
	}
}

func TestRefillMsgQuick(t *testing.T) {
	f := func(id, size uint32, tmsg int8) bool {
		wrm := &transport.RefillMsg{
			Messages: []transport.MessageData{
				{ID: id, Size: size, Typemsg: transport.MsgType(tmsg)},
			},
		}

		b, _ := wrm.MarshalBinary()
		rrm := &transport.RefillMsg{}
		err := rrm.UnmarshalBinary(b)
		require.NoError(t, err)

		if !assert.Equal(t, len(wrm.Messages), len(rrm.Messages)) {
			return false
		}

		for i := range wrm.Messages {
			if !assert.Equal(t, wrm.Messages[i], rrm.Messages[i]) {
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}
