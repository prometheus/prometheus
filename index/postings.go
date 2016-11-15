package index

import (
	"io"

	"github.com/boltdb/bolt"
)

type iteratorStoreFunc func(k uint64) (Iterator, error)

func (s iteratorStoreFunc) get(k uint64) (Iterator, error) {
	return s(k)
}

// boltSkiplistCursor implements the skiplistCurosr interface.
//
// TODO(fabxc): benchmark the overhead of a bucket per key.
// It might be more performant to have all skiplists in the same bucket.
//
// 	20k keys, ~10 skiplist entries avg -> 200k keys, 1 bucket vs 20k buckets, 10 keys
//
type boltSkiplistCursor struct {
	// k is currently unused. If the bucket holds entries for more than
	// just a single key, it will be necessary.
	k   uint64
	c   *bolt.Cursor
	bkt *bolt.Bucket
}

func (s *boltSkiplistCursor) next() (DocID, uint64, error) {
	db, pb := s.c.Next()
	if db == nil {
		return 0, 0, io.EOF
	}
	return newDocID(db), decodeUint64(pb), nil
}

func (s *boltSkiplistCursor) seek(k DocID) (DocID, uint64, error) {
	db, pb := s.c.Seek(k.bytes())
	if db == nil {
		db, pb = s.c.Last()
		if db == nil {
			return 0, 0, io.EOF
		}
	}
	did, pid := newDocID(db), decodeUint64(pb)

	if did > k {
		// If the found entry is behind the seeked ID, try the previous
		// entry if it exists. The page it points to contains the range of k.
		dbp, pbp := s.c.Prev()
		if dbp != nil {
			did, pid = newDocID(dbp), decodeUint64(pbp)
		} else {
			// We skipped before the first entry. The cursor is now out of
			// state and subsequent calls to Next() will return nothing.
			// Reset it to the first position.
			s.c.First()
		}
	}
	return did, pid, nil
}

func (s *boltSkiplistCursor) append(d DocID, p uint64) error {
	k, _ := s.c.Last()

	if k != nil && decodeUint64(k) >= uint64(d) {
		return errOutOfOrder
	}

	return s.bkt.Put(encodeUint64(uint64(d)), encodeUint64(p))
}
