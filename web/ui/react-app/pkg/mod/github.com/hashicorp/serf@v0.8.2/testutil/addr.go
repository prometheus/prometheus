package testutil

import (
	"net"
	"sync"
)

var bindLock sync.Mutex
var bindNum byte = 10

// Returns an unused address for binding to for tests.
func GetBindAddr() net.IP {
	bindLock.Lock()
	defer bindLock.Unlock()

	result := net.IPv4(127, 0, 0, bindNum)
	bindNum++
	if bindNum == 0 {
		bindNum = 10
	}

	return result
}
