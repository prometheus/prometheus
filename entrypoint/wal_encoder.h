#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Construct a new WAL Encoder
 *
 * @param args {
 *     shard_id   uint16 // shard number
 *     log_shards uint8  // logarithm to the base 2 of total shards count
 * }
 * @param res {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
void prompp_wal_encoder_ctor(void* args, void* res);

/**
 * @brief Destroy encoder
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
void prompp_wal_encoder_dtor(void* args);

/**
 * @brief Add data to current segment
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 *     hashdex uintptr // pointer to filled hashdex
 * }
 * @param res {
 *     samples            uint32  // number of samples in segment
 *     series             uint32  // number of series in segment
 *     earliest_timestamp int64   // minimal sample timestamp in segment
 *     latest_timestamp   int64   // maximal sample timestamp in segment
 *     remainder_size     uint32  // rest of internal buffers capacity
 *     error              []byte  // error string if thrown
 * }
 */
void prompp_wal_encoder_add(void* args, void* res);

/**
 * @brief Add data to current segment and mark as stale obsolete series
 *
 * @param args {
 *     encoder      uintptr // pointer to constructed encoder
 *     hashdex      uintptr // pointer to filled hashdex
 *     stale_ts     int64   // timestamp for StaleNaNs
 *     source_state uintptr // pointer to source state (null on first call)
 * }
 * @param res {
 *     samples            uint32  // number of samples in segment
 *     series             uint32  // number of series in segment
 *     earliest_timestamp int64   // minimal sample timestamp in segment
 *     latest_timestamp   int64   // maximal sample timestamp in segment
 *     remainder_size     uint32  // rest of internal buffers capacity
 *     source_state       uintptr // pointer to internal source state
 *     error              []byte  // error string if thrown
 * }
 */
void prompp_wal_encoder_add_with_stale_nans(void* args, void* res);

/**
 * @brief Destroy source state and mark all series as stale
 *
 * @param args {
 *     encoder      uintptr // pointer to constructed encoder
 *     stale_ts     int64   // timestamp for StaleNaNs
 *     source_state uintptr // pointer to source state (null on first call)
 * }
 * @param res {
 *     samples            uint32  // number of samples in segment
 *     series             uint32  // number of series in segment
 *     earliest_timestamp int64   // minimal sample timestamp in segment
 *     latest_timestamp   int64   // maximal sample timestamp in segment
 *     remainder_size     uint32  // rest of internal buffers capacity
 *     error              []byte  // error string if thrown
 * }
 */
void prompp_wal_encoder_collect_source(void* args, void* res);

/**
 * @brief Flush segment
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 * }
 * @param res {
 *     samples            uint32  // number of samples in segment
 *     series             uint32  // number of series in segment
 *     earliest_timestamp int64   // minimal sample timestamp in segment
 *     latest_timestamp   int64   // maximal sample timestamp in segment
 *     remainder_size     uint32  // rest of internal buffers capacity
 *     segment            []byte  // segment content
 *     error              []byte  // error string if thrown
 * }
 */
void prompp_wal_encoder_finalize(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
