#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Basic encoder version
 *
 * @param res {
 *     encoders_version uint8_t // basic encoders version
 * }
 */
void prompp_wal_encoders_version(void* res);

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
 *     encoder uintptr      // pointer to constructed encoder
 *     hashdex uintptr      // pointer to filled hashdex
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
 * @brief Add inner series to current segment
 *
 * @param args {
 *     incoming_inner_series []*InnerSeries // go slice with incoming InnerSeries;
 *     encoder               uintptr        // pointer to constructed encoder;
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
void prompp_wal_encoder_add_inner_series(void* args, void* res);

/**
 * @brief Add relabeled series to current segment
 *
 * @param args {
 *     incoming_relabeled_series []*RelabeledSeries // go slice with incoming RelabeledSeries;
 *     encoder                   uintptr            // pointer to constructed encoder
 *     relabeler_state_update    uintptr            // pointer to constructed RelabelerStateUpdate;
 * }
 * @param res {
 *     earliest_timestamp int64   // minimal sample timestamp in segment
 *     latest_timestamp   int64   // maximal sample timestamp in segment
 *     allocated_memory   uint64  // size of allocated memory for label sets;
 *     samples            uint32  // number of samples in segment
 *     series             uint32  // number of series in segment
 *     remainder_size     uint32  // rest of internal buffers capacity
 *     error              []byte  // error string if thrown
 * }
 */
void prompp_wal_encoder_add_relabeled_series(void* args, void* res);

/**
 * @brief Add data to current segment and mark as stale obsolete series
 *
 * @param args {
 *     encoder      uintptr // pointer to constructed encoder
 *     hashdex      uintptr // pointer to filled hashdex
 *     hashdex_type uint8   // type of hashdex
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

//
// EncoderLightweight
//

/**
 * @brief Construct a new WAL EncoderLightweight
 *
 * @param args {
 *     shardID            uint16  // shard number
 *     logShards          uint8   // logarithm to the base 2 of total shards count
 * }
 * @param res {
 *     encoderLightweight uintptr // pointer to constructed encoder
 * }
 */
void prompp_wal_encoder_lightweight_ctor(void* args, void* res);

/**
 * @brief Destroy EncoderLightweight
 *
 * @param args {
 *     encoderLightweight uintptr // pointer to constructed encoder
 * }
 */
void prompp_wal_encoder_lightweight_dtor(void* args);

/**
 * @brief Add data to current segment
 *
 * @param args {
 *     encoderLightweight uintptr      // pointer to constructed encoder
 *     hashdex            uintptr      // pointer to filled hashdex
 * }
 * @param res {
 *     earliestTimestamp  int64        // minimal sample timestamp in segment
 *     latestTimestamp    int64        // maximal sample timestamp in segment
 *     allocatedMemory    uint64       // size of allocated memory for label sets;
 *     samples            uint32       // number of samples in segment
 *     series             uint32       // number of series in segment
 *     remainderSize      uint32       // rest of internal buffers capacity
 *     error              []byte       // error string if thrown
 * }
 */
void prompp_wal_encoder_lightweight_add(void* args, void* res);

/**
 * @brief Add inner series to current segment
 *
 * @param args {
 *     incomingInnerSeries []*InnerSeries // go slice with incoming InnerSeries;
 *     encoderLightweight  uintptr        // pointer to constructed encoder;
 * }
 * @param res {
 *     earliestTimestamp   int64          // minimal sample timestamp in segment
 *     latestTimestamp     int64          // maximal sample timestamp in segment
 *     allocatedMemory     uint64         // size of allocated memory for label sets;
 *     samples             uint32         // number of samples in segment
 *     series              uint32         // number of series in segment
 *     remainderSize       uint32         // rest of internal buffers capacity
 *     error               []byte         // error string if thrown
 * }
 */
void prompp_wal_encoder_lightweight_add_inner_series(void* args, void* res);

/**
 * @brief Add relabeled series to current segment
 *
 * @param args {
 *     incomingRelabeledSeries []*RelabeledSeries // go slice with incoming RelabeledSeries;
 *     encoderLightweight      uintptr            // pointer to constructed encoder
 *     relabelerStateUpdate    uintptr            // pointer to constructed RelabelerStateUpdate;
 * }
 * @param res {
 *     earliestTimestamp       int64              // minimal sample timestamp in segment
 *     latestTimestamp         int64              // maximal sample timestamp in segment
 *     allocatedMemory         uint64             // size of allocated memory for label sets;
 *     samples                 uint32             // number of samples in segment
 *     series                  uint32             // number of series in segment
 *     remainderSize           uint32             // rest of internal buffers capacity
 *     error                   []byte             // error string if thrown
 * }
 */
void prompp_wal_encoder_lightweight_add_relabeled_series(void* args, void* res);

/**
 * @brief Flush segment
 *
 * @param args {
 *     encoderLightweight uintptr // pointer to constructed encoder
 * }
 * @param res {
 *     earliestTimestamp  int64   // minimal sample timestamp in segment
 *     latestTimestamp    int64   // maximal sample timestamp in segment
 *     allocatedMemory    uint64  // size of allocated memory for label sets;
 *     samples            uint32  // number of samples in segment
 *     series             uint32  // number of series in segment
 *     remainderSize      uint32  // rest of internal buffers capacity
 *     error              []byte  // error string if thrown
 * }
 */
void prompp_wal_encoder_lightweight_finalize(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
