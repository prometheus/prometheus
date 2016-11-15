package tsdb

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"time"

	"github.com/boltdb/bolt"
	"github.com/fabxc/pagebuf"
	"github.com/prometheus/common/log"
)

type persistence struct {
	*chunkBatchProcessor

	mc     *memChunks
	chunks *pagebuf.DB
	index  *bolt.DB
}

func newPersistence(path string, cap int, to time.Duration) (*persistence, error) {
	if err := os.MkdirAll(path, 0777); err != nil {
		return nil, err
	}
	ix, err := bolt.Open(filepath.Join(path, "ix"), 0666, nil)
	if err != nil {
		return nil, err
	}
	if err := ix.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bktChunks)
		return err
	}); err != nil {
		return nil, err
	}
	pb, err := pagebuf.Open(filepath.Join(path, "chunks"), 0666, nil)
	if err != nil {
		return nil, err
	}
	p := &persistence{
		chunks:              pb,
		index:               ix,
		chunkBatchProcessor: newChunkBatchProcessor(log.Base(), cap, to),
	}
	p.chunkBatchProcessor.processf = p.persist

	return p, nil
}

var bktChunks = []byte("chunks")

func (p *persistence) close() error {
	// Index must be closed first, otherwise we might deadlock.
	err0 := p.index.Close()
	err1 := p.chunks.Close()
	if err0 != nil {
		return err0
	}
	return err1
}

func (p *persistence) persist(cds ...*chunkDesc) error {
	err := p.update(func(tx *persistenceTx) error {
		bkt := tx.ix.Bucket(bktChunks)
		for _, cd := range cds {
			pos, err := tx.chunks.Add(cd.chunk.Data())
			if err != nil {
				return err
			}
			var buf [16]byte
			binary.BigEndian.PutUint64(buf[:8], uint64(cd.id))
			binary.BigEndian.PutUint64(buf[8:], pos)
			if err := bkt.Put(buf[:8], buf[8:]); err != nil {
				return err
			}

			tx.ids = append(tx.ids, cd.id)
		}
		return nil
	})
	return err
}

func (p *persistence) update(f func(*persistenceTx) error) error {
	tx, err := p.begin(true)
	if err != nil {
		return err
	}
	if err := f(tx); err != nil {
		tx.rollback()
		return err
	}
	return tx.commit()
}

func (p *persistence) view(f func(*persistenceTx) error) error {
	tx, err := p.begin(false)
	if err != nil {
		return err
	}
	if err := f(tx); err != nil {
		tx.rollback()
		return err
	}
	return tx.rollback()
}

func (p *persistence) begin(writeable bool) (*persistenceTx, error) {
	var err error
	tx := &persistenceTx{p: p}
	// Index transaction is the outer one so we might end up with orphaned
	// chunks but never with dangling pointers in the index.
	tx.ix, err = p.index.Begin(writeable)
	if err != nil {
		return nil, err
	}
	tx.chunks, err = p.chunks.Begin(writeable)
	if err != nil {
		tx.ix.Rollback()
		return nil, err
	}

	return tx, nil
}

type persistenceTx struct {
	p      *persistence
	ix     *bolt.Tx
	chunks *pagebuf.Tx

	ids []ChunkID
}

func (tx *persistenceTx) commit() error {
	if err := tx.chunks.Commit(); err != nil {
		tx.ix.Rollback()
		return err
	}
	if err := tx.ix.Commit(); err != nil {
		// TODO(fabxc): log orphaned chunks. What about overwritten ones?
		// Should we not allows delete and add in the same tx so this cannot happen?
		return err
	}

	// Successfully persisted chunks, clear them from the in-memory
	// forward mapping.
	tx.p.mc.mtx.Lock()
	defer tx.p.mc.mtx.Unlock()

	for _, id := range tx.ids {
		delete(tx.p.mc.chunks, id)
	}
	return nil
}

func (tx *persistenceTx) rollback() error {
	err0 := tx.chunks.Rollback()
	err1 := tx.ix.Rollback()
	if err0 != nil {
		return err0
	}
	return err1
}
