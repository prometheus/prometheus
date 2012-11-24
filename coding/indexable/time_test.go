package indexable

import (
	"math/rand"
	"testing"
	"testing/quick"
	"time"
)

func TestTimeEndToEnd(t *testing.T) {
	tester := func(x int) bool {
		random := rand.New(rand.NewSource(int64(x)))
		buffer := make([]byte, 8)
		incoming := time.Unix(random.Int63(), 0)

		EncodeTimeInto(buffer, incoming)
		outgoing := DecodeTime(buffer)

		return incoming.Equal(outgoing) && incoming.Unix() == outgoing.Unix()
	}

	if err := quick.Check(tester, nil); err != nil {
		t.Error(err)
	}
}
