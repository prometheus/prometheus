package pages

import (
	"fmt"
	"sort"
	"unsafe"
)

// txid represents the internal transaction identifier.
type txid uint64

// Tx represents a read-only or read/write transaction on the page buffer.
// Read-only transactions can be used for retrieving pages.
// Read/write transactions can retrieve and write pages.
//
// IMPORTANT: You must commit or rollback transactions when you are done with
// them. Pages can not be reclaimed by the writer until no more transactions
// are using them. A long running read transaction can cause the database to
// quickly grow.
type Tx struct {
	writable bool
	managed  bool
	db       *DB
	meta     *meta
	pages    map[pgid]*page
	delPages map[pgid]bool

	// WriteFlag specifies the flag for write-related methods like WriteTo().
	// Tx opens the database file with the specified flag to copy the data.
	//
	// By default, the flag is unset, which works well for mostly in-memory
	// workloads. For databases that are much larger than available RAM,
	// set the flag to syscall.O_DIRECT to avoid trashing the page cache.
	WriteFlag int
}

// init initializes the transaction.
func (tx *Tx) init(db *DB) {
	tx.db = db
	tx.pages = nil

	// Copy the meta page since it can be changed by the writer.
	tx.meta = &meta{}
	db.meta().copy(tx.meta)

	// Increment the transaction id and add a page cache for writable transactions.
	if tx.writable {
		tx.pages = make(map[pgid]*page)
		tx.delPages = make(map[pgid]bool)
		tx.meta.txid += txid(1)
	}
}

// ID returns the transaction id.
func (tx *Tx) ID() uint64 {
	return uint64(tx.meta.txid)
}

// Size returns current database size in bytes as seen by this transaction.
func (tx *Tx) Size() int64 {
	return int64(tx.meta.pgid) * int64(tx.db.pageSize)
}

// DB returns a reference to the database that created the transaction.
func (tx *Tx) DB() *DB {
	return tx.db
}

// Writable returns whether the transaction can perform write operations.
func (tx *Tx) Writable() bool {
	return tx.writable
}

// Rollback closes the transaction and ignores all previous updates. Read-only
// transactions must be rolled back and not committed.
func (tx *Tx) Rollback() error {
	_assert(!tx.managed, "managed tx rollback not allowed")
	if tx.db == nil {
		return ErrTxClosed
	}
	tx.rollback()
	return nil
}

func (tx *Tx) rollback() {
	if tx.db == nil {
		return
	}
	if tx.writable {
		tx.db.freelist.rollback(tx.meta.txid)
		tx.db.freelist.reload(tx.db.page(tx.db.meta().freelist))
	}
	tx.close()
}

func (tx *Tx) close() {
	if tx.db == nil {
		return
	}
	if tx.writable {
		// Remove transaction ref & writer lock.
		tx.db.rwtx = nil
		tx.db.rwlock.Unlock()
	} else {
		tx.db.removeTx(tx)
	}

	// Clear all references.
	tx.db = nil
	tx.meta = nil
	tx.pages = nil
}

// page returns a reference to the page with a given id.
// If page has been written to then a temporary buffered page is returned.
func (tx *Tx) page(id pgid) *page {
	// Check the dirty pages first.
	if tx.pages != nil {
		if p, ok := tx.pages[id]; ok {
			return p
		}
	}

	// Otherwise return directly from the mmap.
	return tx.db.page(id)
}

func (tx *Tx) pageExists(id pgid) bool {
	// Check whether the page was modified during this transaction.
	if tx.pages != nil {
		if _, ok := tx.pages[id]; ok {
			return true
		}
	}
	// Check whether page was deleted during this transaction.
	if tx.delPages != nil {
		if tx.delPages[id] {
			return false
		}
	}
	// The page was not touched within this transaction. Fallthrough to
	// the database's check.
	return tx.db.pageExists(id)
}

// allocate returns a contiguous block of memory starting at a given page.
func (tx *Tx) allocate(count int) (*page, error) {
	p, err := tx.db.allocate(count)
	if err != nil {
		return nil, err
	}

	// Save to our page cache.
	tx.pages[p.id] = p
	return p, nil
}

// Commit writes all changes to disk and updates the meta page.
// Returns an error if a disk write error occurs, or if Commit is
// called on a read-only transaction.
func (tx *Tx) Commit() error {
	_assert(!tx.managed, "managed tx commit not allowed")
	if tx.db == nil {
		return ErrTxClosed
	} else if !tx.writable {
		return ErrTxNotWritable
	}

	// TODO(benbjohnson): Use vectorized I/O to write out dirty pages.

	opgid := tx.meta.pgid

	// Free the freelist and allocate new pages for it. This will overestimate
	// the size of the freelist but not underestimate the size (which would be bad).
	tx.db.freelist.free(tx.meta.txid, tx.db.page(tx.meta.freelist))
	p, err := tx.allocate((tx.db.freelist.size() / tx.db.pageSize) + 1)
	if err != nil {
		tx.rollback()
		return err
	}
	if err := tx.db.freelist.write(p); err != nil {
		tx.rollback()
		return err
	}
	tx.meta.freelist = p.id

	// If the high water mark has moved up then attempt to grow the database.
	if tx.meta.pgid > opgid {
		if err := tx.db.grow(int(tx.meta.pgid+1) * tx.db.pageSize); err != nil {
			tx.rollback()
			return err
		}
	}

	// Write dirty pages to disk.
	if err := tx.write(); err != nil {
		tx.rollback()
		return err
	}

	// Write meta to disk.
	if err := tx.writeMeta(); err != nil {
		tx.rollback()
		return err
	}

	// Finalize the transaction.
	tx.close()

	return nil
}

// write writes any dirty pages to disk.
func (tx *Tx) write() error {
	// Sort pages by id.
	pages := make(pages, 0, len(tx.pages))
	for _, p := range tx.pages {
		pages = append(pages, p)
	}
	// Clear out page cache early.
	tx.pages = make(map[pgid]*page)
	sort.Sort(pages)

	// Write pages to disk in order.
	for _, p := range pages {
		size := (int(p.overflow) + 1) * tx.db.pageSize
		offset := int64(p.id) * int64(tx.db.pageSize)

		// Write out page in "max allocation" sized chunks.
		ptr := (*[maxAllocSize]byte)(unsafe.Pointer(p))
		for {
			// Limit our write to our max allocation size.
			sz := size
			if sz > maxAllocSize-1 {
				sz = maxAllocSize - 1
			}

			// Write chunk to disk.
			buf := ptr[:sz]
			if _, err := tx.db.ops.writeAt(buf, offset); err != nil {
				return err
			}

			// Exit inner for loop if we've written all the chunks.
			size -= sz
			if size == 0 {
				break
			}

			// Otherwise move offset forward and move pointer to next chunk.
			offset += int64(sz)
			ptr = (*[maxAllocSize]byte)(unsafe.Pointer(&ptr[sz]))
		}
	}

	if err := fdatasync(tx.db); err != nil {
		return err
	}

	// Put small pages back to page pool.
	for _, p := range pages {
		// Ignore page sizes over 1 page.
		// These are allocated using make() instead of the page pool.
		if int(p.overflow) != 0 {
			continue
		}

		buf := (*[maxAllocSize]byte)(unsafe.Pointer(p))[:tx.db.pageSize]

		// See https://go.googlesource.com/go/+/f03c9202c43e0abb130669852082117ca50aa9b1
		for i := range buf {
			buf[i] = 0
		}
		tx.db.pagePool.Put(buf)
	}

	return nil
}

// writeMeta writes the meta to the disk.
func (tx *Tx) writeMeta() error {
	// Create a temporary buffer for the meta page.
	buf := make([]byte, tx.db.pageSize)
	p := tx.db.pageInBuffer(buf, 0)
	tx.meta.write(p)

	// Write the meta page to file.
	if _, err := tx.db.ops.writeAt(buf, int64(p.id)*int64(tx.db.pageSize)); err != nil {
		return err
	}
	if err := fdatasync(tx.db); err != nil {
		return err
	}

	return nil
}

// Get retrieves the bytes stored in the page with the given id.
// The returned byte slice is only valid for the duration of the transaction.
func (tx *Tx) Get(id uint64) ([]byte, error) {
	if !tx.pageExists(pgid(id)) {
		return nil, ErrNotFound
	}

	p := tx.page(pgid(id))
	size := int(p.overflow)*tx.db.pageSize - PageHeaderSize + int(p.count)
	b := (*[maxAllocSize]byte)(unsafe.Pointer(&p.ptr))[:size]

	return b, nil
}

// Add creates a new page with the given content. The inserted byte slice
// will be padded at the end to fit the next largest page size. Retrieving the page
// will return the padding as well.
// Inserted data should hence have included termination markers.
func (tx *Tx) Add(c []byte) (uint64, error) {
	l := len(c) + PageHeaderSize // total size required
	n := 1                       // number of pages required
	for n*tx.db.pageSize < l {
		n++
	}
	if l > maxAllocSize {
		return 0, fmt.Errorf("page of size %d too large", l)
	}
	p, err := tx.allocate(n)
	if err != nil {
		return 0, fmt.Errorf("page alloc error: %s", err)
	}
	p.flags |= pageFlagData
	// count holds the length used in the last page.
	p.count = uint16(l - (n-1)*tx.db.pageSize)

	b := (*[maxAllocSize]byte)(unsafe.Pointer(&p.ptr))[:]
	copy(b, c)

	return uint64(p.id), nil
}

// Del deletes the page witht he given ID.
func (tx *Tx) Del(id uint64) error {
	if !tx.pageExists(pgid(id)) {
		return ErrNotFound
	}

	tx.db.freelist.free(tx.meta.txid, tx.db.page(pgid(id)))
	return nil
}

// Set overwrites the page with the given ID with c.
func (tx *Tx) Set(id uint64, c []byte) error {
	if !tx.pageExists(pgid(id)) {
		return ErrNotFound
	}
	p := tx.db.page(pgid(id))

	l := len(c) + PageHeaderSize // total size required
	n := int(p.overflow + 1)
	// The contents must fit into the previously allocated pages.
	if l > n*tx.db.pageSize {
		return fmt.Errorf("invalid overwrite size")
	}

	// Allocate a temporary buffer for the page.
	var buf []byte
	if n == 1 {
		buf = tx.db.pagePool.Get().([]byte)
	} else {
		buf = make([]byte, n*tx.db.pageSize)
	}
	np := tx.db.pageInBuffer(buf, 0)
	*np = *p
	// count holds the length used in the last page.
	np.count = uint16(l - (n-1)*tx.db.pageSize)

	// TODO(fabxc): Potential performance improvement point could be using c directly.
	// Just copy it for now.
	b := (*[maxAllocSize]byte)(unsafe.Pointer(&np.ptr))[:]
	copy(b, c)

	tx.pages[pgid(id)] = np
	// TODO(fabxc): truncate and free pages that are no longer needed.

	return nil
}
