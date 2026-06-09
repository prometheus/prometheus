# Agent Result

## Root Cause

`TestDBReadOnly_Querier_NoAlteration/doesn't_truncate_corrupted_chunks` fails on Windows because `repairLastChunkFile` (called from `chunks.NewChunkDiskMapper` -> `openMMapFiles`) tries to delete a corrupted chunk file and Windows returns "The process cannot access the file because it is being used by another process."

The read-only DB path (`loadDataAsQueryable`) uses `chunks.HardLinkChunkFiles` to create hardlinks of the source `chunks_head/` files into a sandbox directory, then opens a `Head` pointing at the sandbox. When `repairLastChunkFile` opens, reads, and closes the sandbox hardlink before trying to delete it, Windows can transiently hold the file locked (hardlinks share the same underlying NTFS object), so `os.RemoveAll` fails.

Because the head is operating on a temporary sandbox that is discarded when the read-only DB closes, a deletion failure is non-fatal: the corrupted file has no valid data to read, and it will be cleaned up when the sandbox directory is removed. A writable DB must still fail hard on deletion errors to prevent sequence-number corruption.

## Change Made

Three files were changed to introduce a `readOnly` repair-tolerance mode that is activated only for the sandbox heads in `(*DBReadOnly).loadDataAsQueryable`.

### `tsdb/chunks/head_chunks.go`

- Added `ChunkDiskMapperOption` functional-option type and `chunks.WithReadOnly()` option constructor.
- Added `readOnly bool` field to `ChunkDiskMapper` struct.
- Changed `chunks.NewChunkDiskMapper` signature to accept a variadic `...ChunkDiskMapperOption` (backward compatible - existing callers pass nothing).
- Options are applied to `m` before `m.openMMapFiles()` runs.
- `openMMapFiles` passes `cdm.readOnly` to `repairLastChunkFile`.
- `repairLastChunkFile(files map[int]string, readOnly bool)` - when `readOnly` is true and `os.RemoveAll` fails, the corrupted file is still removed from the returned `files` map (so it is never mmapped) but no error is returned. When `readOnly` is false the behavior is unchanged.

### `tsdb/head.go`

- Added `ReadOnly bool` field to `HeadOptions`. Zero value (false) preserves all existing behavior.
- In `NewHead`, if `opts.ReadOnly` is set, `chunks.WithReadOnly()` is appended to the options passed to `chunks.NewChunkDiskMapper`.

### `tsdb/db.go` - `(*DBReadOnly).loadDataAsQueryable`

- Set `opts.ReadOnly = true` on both `HeadOptions` instances created for the sandbox heads (the initial no-WAL head and the WAL-replaying head). `FlushWAL`, which creates a head against `db.dir` directly, is not affected.

## Testing

Go is not available in this environment. The changes are minimal and targeted:
- All existing callers of `NewChunkDiskMapper` pass no variadic options (backward compatible).
- Only `loadDataAsQueryable` sets `ReadOnly = true`, which activates the tolerance only for sandbox heads.
- The writable head path continues to return an error if it cannot delete a corrupted chunk file.
- The failing test `TestDBReadOnly_Querier_NoAlteration/doesn't_truncate_corrupted_chunks` should no longer error out on Windows when `repairLastChunkFile` cannot delete the sandbox hardlink. The subsequent `newTestDB(t, withDir(db.Dir()))` call uses the writable path which can still delete the original corrupted file normally.
- An identical test in `db_append_v2_test.go` is also covered by the same fix.

## Lint

Go toolchain is not available in this environment. The changes follow the existing code style (no new imports required, no unused variables, named returns preserved as-is). The `ChunkDiskMapperOption` type and `WithReadOnly` function are exported consistent with the pattern used elsewhere in the package.
