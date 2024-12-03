#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Construct a new WAL Decoder
 *
 * @param args {
 *     encoder_version uint8_t // basic encoder version
 * }
 *
 * @param res {
 *     decoder uintptr // pointer to constructed decoder
 * }
 */
void prompp_wal_decoder_ctor(void* args, void* res);

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
 *     earliest_block_sample int64 // min timestamp in block
 *     latest_block_sample inte64 // max timestamp in block
 *     protobuf   []byte // decoded RemoteWrite protobuf content
 *     error      []byte // error string if thrown
 * }
 */
void prompp_wal_decoder_decode(void* args, void* res);

/**
 * @brief Decode WAL-segment into BasicDecoderHashdex
 *
 * @param args {
 *     decoder               uintptr // pointer to constructed decoder
 *     segment               []byte  // segment content
 * }
 * @param res {
 *     created_at            int64   // timestamp in ns when data was start writed to encoder
 *     encoded_at            int64   // timestamp in ns when segment was encoded
 *     samples               uint32  // number of samples in segment
 *     series                uint32  // number of series in segment
 *     segment_id            uint32  // processed segment id
 *     earliest_block_sample int64   // min timestamp in block
 *     latest_block_sample   inte64  // max timestamp in block
 *     hashdex               uintptr // pointer to filled hashdex
 *     cluster               string  // value of label cluster from first sample
 *     replica               string  // value of label __replica__ from first sample
 *     error                 []byte  // error string if thrown
 * }
 */
void prompp_wal_decoder_decode_to_hashdex(void* args, void* res);

/**
 * @brief Decode WAL-segment into BasicDecoderHashdex with metadata for injection metrics.
 *
 * @param args {
 *     decoder               uintptr        // pointer to constructed decoder
 *     meta                  *MetaInjection // pointer to metadata for injection metrics.
 *     segment               []byte         // segment content
 * }
 * @param res {
 *     created_at            int64          // timestamp in ns when data was start writed to encoder
 *     encoded_at            int64          // timestamp in ns when segment was encoded
 *     samples               uint32         // number of samples in segment
 *     series                uint32         // number of series in segment
 *     segment_id            uint32         // processed segment id
 *     earliest_block_sample int64          // min timestamp in block
 *     latest_block_sample   inte64         // max timestamp in block
 *     hashdex               uintptr        // pointer to filled hashdex
 *     cluster               string         // value of label cluster from first sample
 *     replica               string         // value of label __replica__ from first sample
 *     error                 []byte         // error string if thrown
 * }
 */
void prompp_wal_decoder_decode_to_hashdex_with_metric_injection(void* args, void* res);

/**
 * @brief Decode WAL-segment and drop decoded data
 *
 * @param args {
 *     decoder uintptr // pointer to constructed decoder
 *     segment []byte  // segment content
 * }
 * @param res {
 *     segment_id uint32  // last decoded segment id
 *     error   []byte     // error string if thrown
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

//
// OutputDecoder
//

/**
 * @brief Construct a new WAL Output Decoder
 *
 * @param args {
 *     external_labels     []Label // slice with external labels;
 *     stateless_relabeler uintptr // pointer to constructed stateless relabeler;
 *     output_lss          uintptr // pointer to constructed output label sets;
 *     encoder_version     uint8_t // basic encoder version
 * }
 *
 * @param res {
 *     decoder uintptr // pointer to constructed output decoder
 * }
 */
void prompp_wal_output_decoder_ctor(void* args, void* res);

/**
 * @brief Destroy output decoder
 *
 * @param args {
 *     decoder             uintptr // pointer to constructed output decoder
 * }
 */
void prompp_wal_output_decoder_dtor(void* args);

/**
 * @brief Dump output decoder state(output_lss and cache) to slice byte.
 *
 * @param args {
 *     decoder             uintptr // pointer to constructed output decoder
 * }
 *
 * @param res {
 *     dump                []byte  // stream dump
 *     error               []byte  // error string if thrown
 * }
 */
void prompp_wal_output_decoder_dump_to(void* args, void* res);

/**
 * @brief Load from dump(slice byte) output decoder state(output_lss and cache).
 *
 * @param args {
 *     dump                []byte  // stream dump
 *     decoder             uintptr // pointer to constructed output decoder
 * }
 *
 * @param res {
 *     error               []byte  // error string if thrown
 * }
 */
void prompp_wal_output_decoder_load_from(void* args, void* res);

/**
 * @brief decode segment to slice RefSample.
 *
 * @param args {
 *     decoder               uintptr     // pointer to constructed output decoder
 *     lower_limit_timestamp int64       // lower limit timestamp
 * }
 *
 * @param res {
 *     max_timestamp         int64       // max timestamp in slice RefSample
 *     ref_samples           []RefSample // slice RefSample
 *     error                 []byte      // error string if thrown
 * }
 */
void prompp_wal_output_decoder_decode(void* args, void* res);

//
// ProtobufEncoder
//

/**
 * @brief Construct a new Protobuf Encoder
 *
 * @param args {
 *     output_lsses        uintptr           // pointer to constructed slice with output label sets;
 * }
 *
 * @param res {
 *     encoder             uintptr           // pointer to constructed Protobuf Encoder
 * }
 */
void prompp_wal_protobuf_encoder_ctor(void* args, void* res);

/**
 * @brief Destroy Protobuf Encoder
 *
 * @param args {
 *     encoder             uintptr           // pointer to constructed Protobuf Encoder
 * }
 */
void prompp_wal_protobuf_encoder_dtor(void* args);

/**
 * @brief encode batch slice ShardRefSamples to snapped protobufs on shards.
 *
 * @param args {
 *     batch               []*ShardRefSample // slice with go pointers to ShardRefSample
 *     out_slices          [][]byte          // slice RefSample
 *     encoder             uintptr           // pointer to constructed output decoder
 * }
 *
 * @param res {
 *     error               []byte            // error string if thrown
 * }
 */
void prompp_wal_protobuf_encoder_encode(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
