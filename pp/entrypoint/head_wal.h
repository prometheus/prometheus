#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Construct a new Head WAL encoder
 *
 * @param args {
 *     shardID            uint16  // shard number
 *     logShards          uint8   // logarithm to the base 2 of total shards count
 *.    lss                uintptr // pointer to lss
 * }
 * @param res {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
void prompp_head_wal_encoder_ctor(void* args, void* res);

/**
 * @brief Create encoder from decoder
 *
 * @param args {
 *     decoder uintptr // pointer to decoder
 * }
 * @param res {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
void prompp_head_wal_encoder_ctor_from_decoder(void* args, void* res);

/**
 * @brief Destroy Encoder
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
void prompp_head_wal_encoder_dtor(void* args);

/**
 * @brief Add inner series to current segment
 *
 * @param args {
 *     incomingInnerSeries []*InnerSeries // go slice with inner series;
 *     encoder  uintptr        // pointer to constructed encoder;
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
void prompp_head_wal_encoder_add_inner_series(void* args, void* res);

/**
 * @brief Flush segment
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 * }
 * @param res {
 *     earliestTimestamp  int64   // minimal sample timestamp in segment
 *     latestTimestamp    int64   // maximal sample timestamp in segment
 *     allocatedMemory    uint64  // size of allocated memory for label sets;
 *     samples            uint32  // number of samples in segment
 *     series             uint32  // number of series in segment
 *     remainderSize      uint32  // rest of internal buffers capacity
 *     segment            []byte  // segment content
 *     error              []byte  // error string if thrown
 * }
 */
void prompp_head_wal_encoder_finalize(void* args, void* res);

/**
 * @brief Construct a new Head WAL Decoder
 *
 * @param args {
 *     lss             uintptr // pointer to lss
 *     encoder_version uint8_t // basic encoder version
 * }
 *
 * @param res {
 *     decoder uintptr // pointer to constructed decoder
 * }
 */
void prompp_head_wal_decoder_ctor(void* args, void* res);

/**
 * @brief Destroy decoder
 *
 * @param args {
 *     decoder uintptr // pointer to constructed decoder
 * }
 */
void prompp_head_wal_decoder_dtor(void* args);

/**
 * @brief Decode WAL-segment into protobuf message
 *
 * @param args {
 *     decoder uintptr // pointer to constructed decoder
 *     segment []byte  // segment content
 *    inner_series *InnerSeries // decoded content
 * }
 * @param res {
 *     created_at int64  // timestamp in ns when data was start writed to encoder
 *     encoded_at int64  // timestamp in ns when segment was encoded
 *     samples    uint32 // number of samples in segment
 *     series     uint32 // number of series in segment
 *     segment_id uint32 // processed segment id
 *     earliest_block_sample int64 // min timestamp in block
 *     latest_block_sample int64 // max timestamp in block
 *     error      []byte // error string if thrown
 * }
 */
void prompp_head_wal_decoder_decode(void* args, void* res);

/**
 * @brief Decode WAL-segment into DataStorage
 *
 * @param args {
 *     decoder uintptr // pointer to constructed decoder
 *     segment []byte  // segment content
 *     encoder uintptr // pointer to constructed data_storage encoder
 * }
 * @param res {
 *     error      []byte // error string if thrown
 * }
 */
void prompp_head_wal_decoder_decode_to_data_storage(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
