#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Construct a new WAL Decoder
 *
 * @param res {
 *     decoder uintptr // pointer to constructed decoder
 * }
 */
void prompp_wal_decoder_ctor(void* res);

/**
 * @brief Destroy decoder
 *
 * @param args {
 *     decoder uintptr // pointer to constructed decoder
 * }
 */
void prompp_wal_decoder_dtor(void* args);

/**
 * @brief Decode WAL-segment into protobuf message
 *
 * @param args {
 *     decoder uintptr // pointer to constructed decoder
 *     segment []byte  // segment content
 * }
 * @param res {
 *     created_at int64  // timestamp in ns when data was start writed to encoder
 *     encoded_at int64  // timestamp in ns when segment was encoded
 *     samples    uint32 // number of samples in segment
 *     series     uint32 // number of series in segment
 *     segment_id uint32 // processed segment id
 *     protobuf   []byte // decoded RemoteWrite protobuf content
 *     error      []byte // error string if thrown
 * }
 */
void prompp_wal_decoder_decode(void* args, void* res);

/**
 * @brief Decode WAL-segment and drop decoded data
 *
 * @param args {
 *     decoder uintptr // pointer to constructed decoder
 *     segment []byte  // segment content
 * }
 * @param args {
 *     error   []byte  // error string if thrown
 * }
 */
void prompp_wal_decoder_decode_dry(void* args, void* res);

/**
 * @brief Decode all segments from given stream dump
 *
 * @param args {
 *     decoder    uintptr // pointer to constructed decoder
 *     stream     []byte  // stream dump
 *     segment_id uint32  // id of last segment to decode
 * }
 * @param res {
 *     offset     uint64 // number of read bytes from dump
 *     segment_id uint32 // last decoded segment id
 *     error      []byte // error string if thrown
 * }
 */
void prompp_wal_decoder_restore_from_stream(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
