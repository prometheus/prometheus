package frankenstein

// Based on https://raw.githubusercontent.com/stathat/consistent/master/consistent.go

import (
	"errors"
	"sort"
	"strconv"
	"sync"
)

type uint64s []uint64

func (x uint64s) Len() int           { return len(x) }
func (x uint64s) Less(i, j int) bool { return x[i] < x[j] }
func (x uint64s) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }

// ErrEmptyRing is the error returned when trying to get an element when nothing has been added to hash.
var ErrEmptyRing = errors.New("empty circle")

// Ring holds the information about the members of the consistent hash circle.
type Ring struct {
	sync.RWMutex

	circle       map[uint64]collector
	sortedHashes uint64s
	count        int64
	scratch      [64]byte
}

// NewRing creates a new Ring object with a default setting of 20 replicas for each entry.
//
// To change the number of replicas, set NumberOfReplicas before adding entries.
func NewRing() *Ring {
	return &Ring{
		circle: map[uint64]collector{},
	}
}

// eltKey generates a string key for an element with an index.
func (c *Ring) eltKey(elt string, idx int) string {
	// return elt + "|" + strconv.Itoa(idx)
	return strconv.Itoa(idx) + elt
}

// Add inserts a string element in the consistent hash.
func (c *Ring) Update(col collector) {
	c.Lock()
	defer c.Unlock()
	for _, token := range col.tokens {
		c.circle[token] = col
	}
	c.updateSortedHashes()
}

// Get returns an element close to the hash in the circle.
func (c *Ring) Get(key uint64) (collector, error) {
	c.RLock()
	defer c.RUnlock()
	if len(c.circle) == 0 {
		return collector{}, ErrEmptyRing
	}
	i := c.search(key)
	return c.circle[c.sortedHashes[i]], nil
}

func (c *Ring) search(key uint64) (i int) {
	f := func(x int) bool {
		return c.sortedHashes[x] > key
	}
	i = sort.Search(len(c.sortedHashes), f)
	if i >= len(c.sortedHashes) {
		i = 0
	}
	return
}

func (c *Ring) updateSortedHashes() {
	hashes := c.sortedHashes[:0]
	//reallocate if we're holding on to too much (1/4th)
	if cap(c.sortedHashes) < len(c.circle) {
		hashes = nil
	}
	for k := range c.circle {
		hashes = append(hashes, k)
	}
	sort.Sort(hashes)
	c.sortedHashes = hashes
}
