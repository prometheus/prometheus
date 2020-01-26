package testutil

import (
	"time"
)

func Yield() {
	time.Sleep(10 * time.Millisecond)
}
