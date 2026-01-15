<!-- Code generated from semantic convention specification. DO NOT EDIT. -->

# Metrics

This document describes the metrics defined in this semantic convention registry.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `prometheus_tsdb_blocks_loaded` | gauge | {block} | Number of currently loaded data blocks. |
| `prometheus_tsdb_checkpoint_creations_failed_total` | counter | {checkpoint} | Total number of checkpoint creations that failed. |
| `prometheus_tsdb_checkpoint_creations_total` | counter | {checkpoint} | Total number of checkpoint creations attempted. |
| `prometheus_tsdb_checkpoint_deletions_failed_total` | counter | {checkpoint} | Total number of checkpoint deletions that failed. |
| `prometheus_tsdb_checkpoint_deletions_total` | counter | {checkpoint} | Total number of checkpoint deletions attempted. |
| `prometheus_tsdb_clean_start` | gauge | 1 | Set to 1 if the TSDB was clean at startup, 0 otherwise. |
| `prometheus_tsdb_compaction_chunk_range_seconds` | histogram | s | Final time range of chunks on their first compaction. |
| `prometheus_tsdb_compaction_chunk_samples` | histogram | {sample} | Final number of samples on their first compaction. |
| `prometheus_tsdb_compaction_chunk_size_bytes` | histogram | By | Final size of chunks on their first compaction. |
| `prometheus_tsdb_compaction_duration_seconds` | histogram | s | Duration of compaction runs. |
| `prometheus_tsdb_compaction_populating_block` | gauge | 1 | Set to 1 when a block is being written to the disk. |
| `prometheus_tsdb_compactions_failed_total` | counter | {compaction} | Total number of compactions that failed. |
| `prometheus_tsdb_compactions_skipped_total` | counter | {compaction} | Total number of skipped compactions due to overlap. |
| `prometheus_tsdb_compactions_total` | counter | {compaction} | Total number of compactions that were executed. |
| `prometheus_tsdb_compactions_triggered_total` | counter | {compaction} | Total number of triggered compactions. |
| `prometheus_tsdb_data_replay_duration_seconds` | gauge | s | Time taken to replay the data on disk. |
| `prometheus_tsdb_exemplar_exemplars_appended_total` | counter | {exemplar} | Total number of appended exemplars. |
| `prometheus_tsdb_exemplar_exemplars_in_storage` | gauge | {exemplar} | Number of exemplars currently in circular storage. |
| `prometheus_tsdb_exemplar_last_exemplars_timestamp_seconds` | gauge | s | The timestamp of the oldest exemplar stored in circular storage. |
| `prometheus_tsdb_exemplar_max_exemplars` | gauge | {exemplar} | Total number of exemplars the exemplar storage can store. |
| `prometheus_tsdb_exemplar_out_of_order_exemplars_total` | counter | {exemplar} | Total number of out-of-order exemplar ingestion failed attempts. |
| `prometheus_tsdb_exemplar_series_with_exemplars_in_storage` | gauge | {series} | Number of series with exemplars currently in circular storage. |
| `prometheus_tsdb_head_active_appenders` | gauge | {appender} | Number of currently active appender transactions. |
| `prometheus_tsdb_head_chunks` | gauge | {chunk} | Total number of chunks in the head block. |
| `prometheus_tsdb_head_chunks_created_total` | counter | {chunk} | Total number of chunks created in the head block. |
| `prometheus_tsdb_head_chunks_removed_total` | counter | {chunk} | Total number of chunks removed from the head block. |
| `prometheus_tsdb_head_chunks_storage_size_bytes` | gauge | By | Size of the chunks_head directory. |
| `prometheus_tsdb_head_gc_duration_seconds` | histogram | s | Runtime of garbage collection in the head block. |
| `prometheus_tsdb_head_max_time` | gauge | 1 | Maximum timestamp of the head block. |
| `prometheus_tsdb_head_max_time_seconds` | gauge | s | Maximum timestamp of the head block in seconds. |
| `prometheus_tsdb_head_min_time` | gauge | 1 | Minimum timestamp of the head block. |
| `prometheus_tsdb_head_min_time_seconds` | gauge | s | Minimum timestamp of the head block in seconds. |
| `prometheus_tsdb_head_out_of_order_samples_appended_total` | counter | {sample} | Total number of appended out-of-order samples. |
| `prometheus_tsdb_head_samples_appended_total` | counter | {sample} | Total number of appended samples. |
| `prometheus_tsdb_head_series` | gauge | {series} | Total number of series in the head block. |
| `prometheus_tsdb_head_series_created_total` | counter | {series} | Total number of series created in the head block. |
| `prometheus_tsdb_head_series_not_found_total` | counter | {request} | Total number of requests for series that were not found. |
| `prometheus_tsdb_head_series_removed_total` | counter | {series} | Total number of series removed from the head block. |
| `prometheus_tsdb_head_stale_series` | gauge | {series} | Number of stale series in the head block. |
| `prometheus_tsdb_head_truncations_failed_total` | counter | {truncation} | Total number of head truncations that failed. |
| `prometheus_tsdb_head_truncations_total` | counter | {truncation} | Total number of head truncations attempted. |
| `prometheus_tsdb_isolation_high_watermark` | gauge | 1 | The isolation high watermark. |
| `prometheus_tsdb_isolation_low_watermark` | gauge | 1 | The isolation low watermark. |
| `prometheus_tsdb_lowest_timestamp` | gauge | 1 | Lowest timestamp value stored in the database. |
| `prometheus_tsdb_lowest_timestamp_seconds` | gauge | s | Lowest timestamp value stored in the database in seconds. |
| `prometheus_tsdb_mmap_chunk_corruptions_total` | counter | {corruption} | Total number of memory-mapped chunk corruptions. |
| `prometheus_tsdb_mmap_chunks_total` | counter | {chunk} | Total number of memory-mapped chunks. |
| `prometheus_tsdb_out_of_bound_samples_total` | counter | {sample} | Total number of out-of-bound samples ingestion failed attempts. |
| `prometheus_tsdb_out_of_order_samples_total` | counter | {sample} | Total number of out-of-order samples ingestion failed attempts. |
| `prometheus_tsdb_out_of_order_wbl_completed_pages_total` | counter | {page} | Total number of completed WBL pages for out-of-order samples. |
| `prometheus_tsdb_out_of_order_wbl_fsync_duration_seconds` | histogram | s | Duration of WBL fsync for out-of-order samples. |
| `prometheus_tsdb_out_of_order_wbl_page_flushes_total` | counter | {flush} | Total number of WBL page flushes for out-of-order samples. |
| `prometheus_tsdb_out_of_order_wbl_record_part_writes_total` | counter | {write} | Total number of WBL record part writes for out-of-order samples. |
| `prometheus_tsdb_out_of_order_wbl_record_parts_bytes_written_total` | counter | By | Total bytes written to WBL record parts for out-of-order samples. |
| `prometheus_tsdb_out_of_order_wbl_segment_current` | gauge | {segment} | Current out-of-order WBL segment. |
| `prometheus_tsdb_out_of_order_wbl_storage_size_bytes` | gauge | By | Size of the out-of-order WBL storage. |
| `prometheus_tsdb_out_of_order_wbl_truncations_failed_total` | counter | {truncation} | Total number of out-of-order WBL truncations that failed. |
| `prometheus_tsdb_out_of_order_wbl_truncations_total` | counter | {truncation} | Total number of out-of-order WBL truncations. |
| `prometheus_tsdb_out_of_order_wbl_writes_failed_total` | counter | {write} | Total number of out-of-order WBL writes that failed. |
| `prometheus_tsdb_reloads_failures_total` | counter | {reload} | Number of times the database reloads failed. |
| `prometheus_tsdb_reloads_total` | counter | {reload} | Number of times the database reloads. |
| `prometheus_tsdb_retention_limit_bytes` | gauge | By | Maximum number of bytes to be retained in the TSDB. |
| `prometheus_tsdb_retention_limit_seconds` | gauge | s | Maximum age in seconds for samples to be retained in the TSDB. |
| `prometheus_tsdb_sample_ooo_delta` | histogram | s | Delta in seconds between the time when an out-of-order sample was ingested and the latest sample in the chunk. |
| `prometheus_tsdb_size_retentions_total` | counter | {retention} | Number of times that blocks were deleted because the maximum number of bytes was exceeded. |
| `prometheus_tsdb_snapshot_replay_error_total` | counter | {error} | Total number of snapshot replay errors. |
| `prometheus_tsdb_storage_blocks_bytes` | gauge | By | The number of bytes that are currently used for local storage by all blocks. |
| `prometheus_tsdb_symbol_table_size_bytes` | gauge | By | Size of the symbol table in bytes. |
| `prometheus_tsdb_time_retentions_total` | counter | {retention} | Number of times that blocks were deleted because the maximum time limit was exceeded. |
| `prometheus_tsdb_tombstone_cleanup_seconds` | histogram | s | Time taken to clean up tombstones. |
| `prometheus_tsdb_too_old_samples_total` | counter | {sample} | Total number of samples that were too old to be ingested. |
| `prometheus_tsdb_vertical_compactions_total` | counter | {compaction} | Total number of compactions done on overlapping blocks. |
| `prometheus_tsdb_wal_completed_pages_total` | counter | {page} | Total number of completed WAL pages. |
| `prometheus_tsdb_wal_corruptions_total` | counter | {corruption} | Total number of WAL corruptions. |
| `prometheus_tsdb_wal_fsync_duration_seconds` | histogram | s | Duration of WAL fsync. |
| `prometheus_tsdb_wal_page_flushes_total` | counter | {flush} | Total number of WAL page flushes. |
| `prometheus_tsdb_wal_record_bytes_saved_total` | counter | By | Total bytes saved by WAL record compression. |
| `prometheus_tsdb_wal_record_part_writes_total` | counter | {write} | Total number of WAL record part writes. |
| `prometheus_tsdb_wal_record_parts_bytes_written_total` | counter | By | Total bytes written to WAL record parts. |
| `prometheus_tsdb_wal_segment_current` | gauge | {segment} | Current WAL segment. |
| `prometheus_tsdb_wal_storage_size_bytes` | gauge | By | Size of the WAL storage. |
| `prometheus_tsdb_wal_truncate_duration_seconds` | histogram | s | Duration of WAL truncation. |
| `prometheus_tsdb_wal_truncations_failed_total` | counter | {truncation} | Total number of WAL truncations that failed. |
| `prometheus_tsdb_wal_truncations_total` | counter | {truncation} | Total number of WAL truncations. |
| `prometheus_tsdb_wal_writes_failed_total` | counter | {write} | Total number of WAL writes that failed. |


## Metric Details


### `prometheus_tsdb_blocks_loaded`

Number of currently loaded data blocks.

- **Type:** gauge
- **Unit:** {block}
- **Stability:** development


### `prometheus_tsdb_checkpoint_creations_failed_total`

Total number of checkpoint creations that failed.

- **Type:** counter
- **Unit:** {checkpoint}
- **Stability:** development


### `prometheus_tsdb_checkpoint_creations_total`

Total number of checkpoint creations attempted.

- **Type:** counter
- **Unit:** {checkpoint}
- **Stability:** development


### `prometheus_tsdb_checkpoint_deletions_failed_total`

Total number of checkpoint deletions that failed.

- **Type:** counter
- **Unit:** {checkpoint}
- **Stability:** development


### `prometheus_tsdb_checkpoint_deletions_total`

Total number of checkpoint deletions attempted.

- **Type:** counter
- **Unit:** {checkpoint}
- **Stability:** development


### `prometheus_tsdb_clean_start`

Set to 1 if the TSDB was clean at startup, 0 otherwise.

- **Type:** gauge
- **Unit:** 1
- **Stability:** development


### `prometheus_tsdb_compaction_chunk_range_seconds`

Final time range of chunks on their first compaction.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_tsdb_compaction_chunk_samples`

Final number of samples on their first compaction.

- **Type:** histogram
- **Unit:** {sample}
- **Stability:** development


### `prometheus_tsdb_compaction_chunk_size_bytes`

Final size of chunks on their first compaction.

- **Type:** histogram
- **Unit:** By
- **Stability:** development


### `prometheus_tsdb_compaction_duration_seconds`

Duration of compaction runs.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_tsdb_compaction_populating_block`

Set to 1 when a block is being written to the disk.

- **Type:** gauge
- **Unit:** 1
- **Stability:** development


### `prometheus_tsdb_compactions_failed_total`

Total number of compactions that failed.

- **Type:** counter
- **Unit:** {compaction}
- **Stability:** development


### `prometheus_tsdb_compactions_skipped_total`

Total number of skipped compactions due to overlap.

- **Type:** counter
- **Unit:** {compaction}
- **Stability:** development


### `prometheus_tsdb_compactions_total`

Total number of compactions that were executed.

- **Type:** counter
- **Unit:** {compaction}
- **Stability:** development


### `prometheus_tsdb_compactions_triggered_total`

Total number of triggered compactions.

- **Type:** counter
- **Unit:** {compaction}
- **Stability:** development


### `prometheus_tsdb_data_replay_duration_seconds`

Time taken to replay the data on disk.

- **Type:** gauge
- **Unit:** s
- **Stability:** development


### `prometheus_tsdb_exemplar_exemplars_appended_total`

Total number of appended exemplars.

- **Type:** counter
- **Unit:** {exemplar}
- **Stability:** development


### `prometheus_tsdb_exemplar_exemplars_in_storage`

Number of exemplars currently in circular storage.

- **Type:** gauge
- **Unit:** {exemplar}
- **Stability:** development


### `prometheus_tsdb_exemplar_last_exemplars_timestamp_seconds`

The timestamp of the oldest exemplar stored in circular storage.

- **Type:** gauge
- **Unit:** s
- **Stability:** development


### `prometheus_tsdb_exemplar_max_exemplars`

Total number of exemplars the exemplar storage can store.

- **Type:** gauge
- **Unit:** {exemplar}
- **Stability:** development


### `prometheus_tsdb_exemplar_out_of_order_exemplars_total`

Total number of out-of-order exemplar ingestion failed attempts.

- **Type:** counter
- **Unit:** {exemplar}
- **Stability:** development


### `prometheus_tsdb_exemplar_series_with_exemplars_in_storage`

Number of series with exemplars currently in circular storage.

- **Type:** gauge
- **Unit:** {series}
- **Stability:** development


### `prometheus_tsdb_head_active_appenders`

Number of currently active appender transactions.

- **Type:** gauge
- **Unit:** {appender}
- **Stability:** development


### `prometheus_tsdb_head_chunks`

Total number of chunks in the head block.

- **Type:** gauge
- **Unit:** {chunk}
- **Stability:** development


### `prometheus_tsdb_head_chunks_created_total`

Total number of chunks created in the head block.

- **Type:** counter
- **Unit:** {chunk}
- **Stability:** development


### `prometheus_tsdb_head_chunks_removed_total`

Total number of chunks removed from the head block.

- **Type:** counter
- **Unit:** {chunk}
- **Stability:** development


### `prometheus_tsdb_head_chunks_storage_size_bytes`

Size of the chunks_head directory.

- **Type:** gauge
- **Unit:** By
- **Stability:** development


### `prometheus_tsdb_head_gc_duration_seconds`

Runtime of garbage collection in the head block.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_tsdb_head_max_time`

Maximum timestamp of the head block.

- **Type:** gauge
- **Unit:** 1
- **Stability:** development


### `prometheus_tsdb_head_max_time_seconds`

Maximum timestamp of the head block in seconds.

- **Type:** gauge
- **Unit:** s
- **Stability:** development


### `prometheus_tsdb_head_min_time`

Minimum timestamp of the head block.

- **Type:** gauge
- **Unit:** 1
- **Stability:** development


### `prometheus_tsdb_head_min_time_seconds`

Minimum timestamp of the head block in seconds.

- **Type:** gauge
- **Unit:** s
- **Stability:** development


### `prometheus_tsdb_head_out_of_order_samples_appended_total`

Total number of appended out-of-order samples.

- **Type:** counter
- **Unit:** {sample}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `type` | string | The sample type. | float, histogram |



### `prometheus_tsdb_head_samples_appended_total`

Total number of appended samples.

- **Type:** counter
- **Unit:** {sample}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `type` | string | The sample type. | float, histogram |



### `prometheus_tsdb_head_series`

Total number of series in the head block.

- **Type:** gauge
- **Unit:** {series}
- **Stability:** development


### `prometheus_tsdb_head_series_created_total`

Total number of series created in the head block.

- **Type:** counter
- **Unit:** {series}
- **Stability:** development


### `prometheus_tsdb_head_series_not_found_total`

Total number of requests for series that were not found.

- **Type:** counter
- **Unit:** {request}
- **Stability:** development


### `prometheus_tsdb_head_series_removed_total`

Total number of series removed from the head block.

- **Type:** counter
- **Unit:** {series}
- **Stability:** development


### `prometheus_tsdb_head_stale_series`

Number of stale series in the head block.

- **Type:** gauge
- **Unit:** {series}
- **Stability:** development


### `prometheus_tsdb_head_truncations_failed_total`

Total number of head truncations that failed.

- **Type:** counter
- **Unit:** {truncation}
- **Stability:** development


### `prometheus_tsdb_head_truncations_total`

Total number of head truncations attempted.

- **Type:** counter
- **Unit:** {truncation}
- **Stability:** development


### `prometheus_tsdb_isolation_high_watermark`

The isolation high watermark.

- **Type:** gauge
- **Unit:** 1
- **Stability:** development


### `prometheus_tsdb_isolation_low_watermark`

The isolation low watermark.

- **Type:** gauge
- **Unit:** 1
- **Stability:** development


### `prometheus_tsdb_lowest_timestamp`

Lowest timestamp value stored in the database.

- **Type:** gauge
- **Unit:** 1
- **Stability:** development


### `prometheus_tsdb_lowest_timestamp_seconds`

Lowest timestamp value stored in the database in seconds.

- **Type:** gauge
- **Unit:** s
- **Stability:** development


### `prometheus_tsdb_mmap_chunk_corruptions_total`

Total number of memory-mapped chunk corruptions.

- **Type:** counter
- **Unit:** {corruption}
- **Stability:** development


### `prometheus_tsdb_mmap_chunks_total`

Total number of memory-mapped chunks.

- **Type:** counter
- **Unit:** {chunk}
- **Stability:** development


### `prometheus_tsdb_out_of_bound_samples_total`

Total number of out-of-bound samples ingestion failed attempts.

- **Type:** counter
- **Unit:** {sample}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `type` | string | The sample type. | float |



### `prometheus_tsdb_out_of_order_samples_total`

Total number of out-of-order samples ingestion failed attempts.

- **Type:** counter
- **Unit:** {sample}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `type` | string | The sample type. | float, histogram |



### `prometheus_tsdb_out_of_order_wbl_completed_pages_total`

Total number of completed WBL pages for out-of-order samples.

- **Type:** counter
- **Unit:** {page}
- **Stability:** development


### `prometheus_tsdb_out_of_order_wbl_fsync_duration_seconds`

Duration of WBL fsync for out-of-order samples.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_tsdb_out_of_order_wbl_page_flushes_total`

Total number of WBL page flushes for out-of-order samples.

- **Type:** counter
- **Unit:** {flush}
- **Stability:** development


### `prometheus_tsdb_out_of_order_wbl_record_part_writes_total`

Total number of WBL record part writes for out-of-order samples.

- **Type:** counter
- **Unit:** {write}
- **Stability:** development


### `prometheus_tsdb_out_of_order_wbl_record_parts_bytes_written_total`

Total bytes written to WBL record parts for out-of-order samples.

- **Type:** counter
- **Unit:** By
- **Stability:** development


### `prometheus_tsdb_out_of_order_wbl_segment_current`

Current out-of-order WBL segment.

- **Type:** gauge
- **Unit:** {segment}
- **Stability:** development


### `prometheus_tsdb_out_of_order_wbl_storage_size_bytes`

Size of the out-of-order WBL storage.

- **Type:** gauge
- **Unit:** By
- **Stability:** development


### `prometheus_tsdb_out_of_order_wbl_truncations_failed_total`

Total number of out-of-order WBL truncations that failed.

- **Type:** counter
- **Unit:** {truncation}
- **Stability:** development


### `prometheus_tsdb_out_of_order_wbl_truncations_total`

Total number of out-of-order WBL truncations.

- **Type:** counter
- **Unit:** {truncation}
- **Stability:** development


### `prometheus_tsdb_out_of_order_wbl_writes_failed_total`

Total number of out-of-order WBL writes that failed.

- **Type:** counter
- **Unit:** {write}
- **Stability:** development


### `prometheus_tsdb_reloads_failures_total`

Number of times the database reloads failed.

- **Type:** counter
- **Unit:** {reload}
- **Stability:** development


### `prometheus_tsdb_reloads_total`

Number of times the database reloads.

- **Type:** counter
- **Unit:** {reload}
- **Stability:** development


### `prometheus_tsdb_retention_limit_bytes`

Maximum number of bytes to be retained in the TSDB.

- **Type:** gauge
- **Unit:** By
- **Stability:** development


### `prometheus_tsdb_retention_limit_seconds`

Maximum age in seconds for samples to be retained in the TSDB.

- **Type:** gauge
- **Unit:** s
- **Stability:** development


### `prometheus_tsdb_sample_ooo_delta`

Delta in seconds between the time when an out-of-order sample was ingested and the latest sample in the chunk.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_tsdb_size_retentions_total`

Number of times that blocks were deleted because the maximum number of bytes was exceeded.

- **Type:** counter
- **Unit:** {retention}
- **Stability:** development


### `prometheus_tsdb_snapshot_replay_error_total`

Total number of snapshot replay errors.

- **Type:** counter
- **Unit:** {error}
- **Stability:** development


### `prometheus_tsdb_storage_blocks_bytes`

The number of bytes that are currently used for local storage by all blocks.

- **Type:** gauge
- **Unit:** By
- **Stability:** development


### `prometheus_tsdb_symbol_table_size_bytes`

Size of the symbol table in bytes.

- **Type:** gauge
- **Unit:** By
- **Stability:** development


### `prometheus_tsdb_time_retentions_total`

Number of times that blocks were deleted because the maximum time limit was exceeded.

- **Type:** counter
- **Unit:** {retention}
- **Stability:** development


### `prometheus_tsdb_tombstone_cleanup_seconds`

Time taken to clean up tombstones.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_tsdb_too_old_samples_total`

Total number of samples that were too old to be ingested.

- **Type:** counter
- **Unit:** {sample}
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `type` | string | The sample type. | float |



### `prometheus_tsdb_vertical_compactions_total`

Total number of compactions done on overlapping blocks.

- **Type:** counter
- **Unit:** {compaction}
- **Stability:** development


### `prometheus_tsdb_wal_completed_pages_total`

Total number of completed WAL pages.

- **Type:** counter
- **Unit:** {page}
- **Stability:** development


### `prometheus_tsdb_wal_corruptions_total`

Total number of WAL corruptions.

- **Type:** counter
- **Unit:** {corruption}
- **Stability:** development


### `prometheus_tsdb_wal_fsync_duration_seconds`

Duration of WAL fsync.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_tsdb_wal_page_flushes_total`

Total number of WAL page flushes.

- **Type:** counter
- **Unit:** {flush}
- **Stability:** development


### `prometheus_tsdb_wal_record_bytes_saved_total`

Total bytes saved by WAL record compression.

- **Type:** counter
- **Unit:** By
- **Stability:** development

#### Attributes

| Attribute | Type | Description | Examples |
|-----------|------|-------------|----------|
| `compression` | string | The compression algorithm. | snappy |



### `prometheus_tsdb_wal_record_part_writes_total`

Total number of WAL record part writes.

- **Type:** counter
- **Unit:** {write}
- **Stability:** development


### `prometheus_tsdb_wal_record_parts_bytes_written_total`

Total bytes written to WAL record parts.

- **Type:** counter
- **Unit:** By
- **Stability:** development


### `prometheus_tsdb_wal_segment_current`

Current WAL segment.

- **Type:** gauge
- **Unit:** {segment}
- **Stability:** development


### `prometheus_tsdb_wal_storage_size_bytes`

Size of the WAL storage.

- **Type:** gauge
- **Unit:** By
- **Stability:** development


### `prometheus_tsdb_wal_truncate_duration_seconds`

Duration of WAL truncation.

- **Type:** histogram
- **Unit:** s
- **Stability:** development


### `prometheus_tsdb_wal_truncations_failed_total`

Total number of WAL truncations that failed.

- **Type:** counter
- **Unit:** {truncation}
- **Stability:** development


### `prometheus_tsdb_wal_truncations_total`

Total number of WAL truncations.

- **Type:** counter
- **Unit:** {truncation}
- **Stability:** development


### `prometheus_tsdb_wal_writes_failed_total`

Total number of WAL writes that failed.

- **Type:** counter
- **Unit:** {write}
- **Stability:** development
