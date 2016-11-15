package pages

import (
	"fmt"
	"os"
	"sort"
	"unsafe"
)

const PageHeaderSize = int(unsafe.Offsetof(((*page)(nil)).ptr))

const (
	// pageFlag{head,tail,body}?
	pageFlagMeta     = 0x02
	pageFlagFreelist = 0x04
	pageFlagData     = 0x08
)

type pgid uint64

type page struct {
	id       pgid
	flags    uint16
	count    uint16
	overflow uint32
	ptr      uintptr
}

// typ returns a human readable page type string.
func (p *page) typ() string {
	if (p.flags & pageFlagMeta) != 0 {
		return "meta"
	} else if (p.flags & pageFlagFreelist) != 0 {
		return "freelist"
	} else if (p.flags & pageFlagData) != 0 {
		return "data"
	}
	return fmt.Sprintf("unknown<%02x>", p.flags)
}

func (p *page) String() string {
	return fmt.Sprintf("page<%s,%016x>", p.typ(), p.id)
}

// meta returns a pointer to the metadata section of a page.
func (p *page) meta() *meta {
	return (*meta)(unsafe.Pointer(&p.ptr))
}

// dump writes n bytes of the page to STDERR as hex output.
func (p *page) hexdump(n int) {
	buf := (*[maxAllocSize]byte)(unsafe.Pointer(p))[:n]
	fmt.Fprintf(os.Stderr, "%x\n", buf)
}

type pages []*page

func (s pages) Len() int           { return len(s) }
func (s pages) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s pages) Less(i, j int) bool { return s[i].id < s[j].id }

type pgids []pgid

func (s pgids) Len() int           { return len(s) }
func (s pgids) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s pgids) Less(i, j int) bool { return s[i] < s[j] }

// merge returns the sorted union of a and b.
func (a pgids) merge(b pgids) pgids {
	// Return the opposite slice if one is nil.
	if len(a) == 0 {
		return b
	} else if len(b) == 0 {
		return a
	}

	// Create a list to hold all elements from both lists.
	merged := make(pgids, 0, len(a)+len(b))

	// Assign lead to the slice with a lower starting value, follow to the higher value.
	lead, follow := a, b
	if b[0] < a[0] {
		lead, follow = b, a
	}

	// Continue while there are elements in the lead.
	for len(lead) > 0 {
		// Merge largest prefix of lead that is ahead of follow[0].
		n := sort.Search(len(lead), func(i int) bool { return lead[i] > follow[0] })
		merged = append(merged, lead[:n]...)
		if n >= len(lead) {
			break
		}

		// Swap lead and follow.
		lead, follow = follow, lead[n:]
	}

	// Append what's left in follow.
	merged = append(merged, follow...)

	return merged
}
