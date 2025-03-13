#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Free memory allocated for response as []byte
 *
 * @param args *[]byte
 */
void prompp_free_bytes(void* args);

/**
 * @brief Return information about using memory by core
 *
 * @param res {
 *   in_use uint64 // bytes in use
 * }
 */
void prompp_mem_info(void* res);

/**
 * @brief Dump jemalloc memory profile to file
 *
 * @param args {
 *     filename string
 * }
 * @param res {
 *     int error
 * }
 */
void prompp_dump_memory_profile(void* args, void* res);

#ifdef __cplusplus
}
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Return head status
 *
 * @param args {
 *     lss         uintptr      // pointer to constructed lss
 *     dataStorage uintptr      // pointer to constructed data storage
 *     limit       int          // statistics limit
 * }
 *
 * @param res {
 *     status struct {     // head status
 *          time_interval struct {
 *              min int64
 *              max int64
 *          }
 *          label_value_count_by_label_name []struct {
 *              name string
 *              count uint32
 *          }
 *          series_count_by_metric_name []struct {
 *              name string
 *              count uint32
 *          }
 *          memory_in_bytes_by_label_name []struct {
 *              name string
 *              size uint32
 *          }
 *          series_count_by_label_value_pair [] struct {
 *              name string
 *              value string
 *              count uint32
 *          }
 *          num_series      uint32
 *          chunk_count     uint32
 *          num_label_pairs uint32
 *          rule_queried_series uint32
 *          federate_queried_series uint32
 *          other_queried_series uint32
 *     }
 * }
 */
void prompp_get_head_status(void* args, void* res);

/**
 * @brief Return head status
 *
 * @param args {
 *     status struct {...} // status returned by prompp_get_head_status
 * }
 *
 */
void prompp_free_head_status(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
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
#pragma once

#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Construct index writer
 *
 * @param args {
 *     lss         uintptr      // pointer to constructed lss
 * }
 * @param res {
 *     writer    uintptr
 * }
 */
void prompp_index_writer_ctor(void* args, void* res);

/**
 * @brief Destroy index writer
 *
 * @param args {
 *     writer    uintptr
 * }
 */
void prompp_index_writer_dtor(void* args);

/**
 * @brief Write header
 *
 * @param args {
 *     writer    uintptr
 * }
 * @param res {
 *     data []byte // only c allocated memory can be re-used
 * }
 */
void prompp_index_writer_write_header(void* args, void* res);

/**
 * @brief Write symbols
 *
 * @param args {
 *     writer    uintptr
 * }
 * @param res {
 *     data []byte // only c allocated memory can be re-used
 * }
 */
void prompp_index_writer_write_symbols(void* args, void* res);

/**
 * @brief Write next series batch
 *
 * @param args {
 *     writer      uintptr
 *     chunks_meta []struct{ // chunks metadata slice
 *         min_t     int64
 *         max_t     int64
 *         reference uint64
 *     }
 *     ls_id       uint32
 * }
 * @param res {
 *     data          []byte // only c allocated memory can be re-used
 * }
 */
void prompp_index_writer_write_next_series_batch(void* args, void* res);

/**
 * @brief Write label indices
 *
 * @param args {
 *     writer    uintptr
 * }
 * @param res {
 *     data []byte // only c allocated memory can be re-used
 * }
 */
void prompp_index_writer_write_label_indices(void* args, void* res);

/**
 * @brief Write next postings batch
 *
 * @param args {
 *     writer         uintptr
 *     max_batch_size uint32
 * }
 * @param res {
 *     data          []byte // only c allocated memory can be re-used
 *     has_more_data bool   // true if we should repeat this call
 * }
 */
void prompp_index_writer_write_next_postings_batch(void* args, void* res);

/**
 * @brief Write label indeces table
 *
 * @param args {
 *     writer    uintptr
 * }
 * @param res {
 *     data []byte // only c allocated memory can be re-used
 * }
 */
void prompp_index_writer_write_label_indices_table(void* args, void* res);

/**
 * @brief Write postings offset table
 *
 * @param args {
 *     writer    uintptr
 * }
 * @param res {
 *     data []byte // only c allocated memory can be re-used
 * }
 */
void prompp_index_writer_write_postings_table_offsets(void* args, void* res);

/**
 * @brief Write table of contents
 *
 * @param args {
 *     writer    uintptr
 * }
 * @param res {
 *     data []byte // only c allocated memory can be re-used
 * }
 */
void prompp_index_writer_write_table_of_contents(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Construct a new Primitives label sets.
 *
 * @param args {
 *     lss_type uint32 // type of lss;
 * }
 *
 * @param res {
 *     lss uintptr     // pointer to constructed label sets;
 * }
 */
void prompp_primitives_lss_ctor(void* args, void* res);

/**
 * @brief Destroy Primitives label sets.
 *
 * @param args {
 *     lss uintptr // pointer of label sets;
 * }
 */
void prompp_primitives_lss_dtor(void* args);

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr             // pointer to constructed label sets;
 * }
 *
 * @param res {
 *     allocated_memory uint64 // size of allocated memory for label sets;
 * }
 */
void prompp_primitives_lss_allocated_memory(void* args, void* res);

/**
 * @brief insert label set into lss
 *
 * @param args {
 *     lss uintptr              // pointer to constructed lss;
 *     label_set model.LabelSet // label set
 * }
 *
 * @param res {
 *     ls_id uint32 // inserted (or found) label set id
 * }
 */
void prompp_primitives_lss_find_or_emplace(void* args, void* res);

/**
 * @brief query series from lss
 *
 * @param args {
 *     lss uintptr                         // pointer to constructed queryable lss;
 *     label_matchers []model.LabelMatcher // label matchers
 *     query_source uint32                 // query source (rule, federate, other)
 * }
 *
 * @param res {
 *     status uint32    // query status
 *     matches []uint32 // matched series ids
 * }
 */
void prompp_primitives_lss_query(void* args, void* res);

/**
 * @brief get label sets by series id
 *
 * @param args {
 *     lss uintptr    // pointer to constructed lss;
 *     ls_id []uint32 // series ids
 * }
 *
 * @param res {
 *     label_sets [][]struct {key, value String} // label sets
 * }
 */
void prompp_primitives_lss_get_label_sets(void* args, void* res);

/**
 * @brief free label sets returned by prompp_primitives_lss_get_label_sets
 *
 * @param args {
 *     label_sets [][]struct {key, value String} // label set
 * }
 */
void prompp_primitives_lss_free_label_sets(void* args);

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr                         // pointer to constructed queryable lss;
 *     label_matchers []model.LabelMatcher // label matchers
 * }
 *
 * @param res {
 *     status uint32   // query status
 *     names  []string // Slice of string freed by freeBytes in GO pointed to lss memory, so it may be invalid after mutating lss state
 * }
 */
void prompp_primitives_lss_query_label_names(void* args, void* res);

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr                         // pointer to constructed queryable lss;
 *     label_name string                   // label name
 *     label_matchers []model.LabelMatcher // label matchers
 * }
 *
 * @param res {
 *     status uint32   // query status
 *     values []string // Slice of string freed by freeBytes in GO pointed to lss memory, so it may be invalid after mutating lss state
 * }
 */
void prompp_primitives_lss_query_label_values(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

//
// StatelessRelabeler
//

/**
 * @brief Construct a new StatelessRelabeler.
 *
 * @param args {
 *     cfgs                []*Config // go slice with pointer RelabelConfig;
 * }
 *
 * @param res {
 *     stateless_relabeler uintptr   // pointer to constructed StatelessRelabeler;
 *     error               []byte    // error string if thrown;
 * }
 */
void prompp_prometheus_stateless_relabeler_ctor(void* args, void* res);

/**
 * @brief Destroy StatelessRelabeler
 *
 * @param args {
 *     stateless_relabeler uintptr // pointer of StatelessRelabeler;
 * }
 */
void prompp_prometheus_stateless_relabeler_dtor(void* args);

/**
 * @brief reset_to reset configs and replace on new converting go-config.
 *
 * @param args {
 *     stateless_relabeler uintptr   // pointer to constructed StatelessRelabeler;
 *     cfgs                []*Config // go slice with pointer RelabelConfig;
 * }
 *
 * @param res {
 *     error               []byte    // error string if thrown;
 * }
 */
void prompp_prometheus_stateless_relabeler_reset_to(void* args, void* res);

//
// InnerSeries
//

/**
 * @brief filling InnerSeries pointer vector InnerSerie;
 *
 * @param args {
 *     innerSeries *InnerSeries // pointer to InnerSeries;
 * }
 */
void prompp_prometheus_inner_series_ctor(void* args);

/**
 * @brief Destroy vector with InnerSerie in InnerSeries.
 *
 * @param args {
 *      innerSeries *InnerSeries // pointer to InnerSeries;
 * }
 */
void prompp_prometheus_inner_series_dtor(void* args);

//
// RelabeledSeries
//

/**
 * @brief filling RelabeledSeries pointer vector RelabeledSerie;
 *
 * @param args {
 *     relabeledSeries *RelabeledSeries // pointer to RelabeledSeries;
 * }
 */
void prompp_prometheus_relabeled_series_ctor(void* args);

/**
 * @brief Destroy vector with RelabeledSerie in RelabeledSeries.
 *
 * @param args {
 *      relabeledSeries *RelabeledSeries // pointer to RelabeledSeries;
 * }
 */
void prompp_prometheus_relabeled_series_dtor(void* args);

//
// RelabelerStateUpdate
//

/**
 * @brief init RelabelerStateUpdate(pointer to RelabelerStateUpdate).
 *
 * @param res {
 *     relabeler_state_update *RelabelerStateUpdate // pointer to RelabelerStateUpdate;
 * }
 */
void prompp_prometheus_relabeler_state_update_ctor(void* args);

/**
 * @brief Destroy vector in RelabelerStateUpdate.
 *
 * @param args {
 *      relabeledSeries *RelabeledSeries // pointer to RelabeledSeries;
 * }
 */
void prompp_prometheus_relabeler_state_update_dtor(void* args);

//
// PerShardRelabeler
//

/**
 * @brief Construct a new PerShardRelabeler.
 *
 * @param args {
 *     external_labels     []Label // slice with external labels;
 *     stateless_relabeler uintptr // pointer to constructed stateless relabeler;
 *     shard_id            uint16  // current shard id;
 *     log_shards          uint8   // logarithm to the base 2 of total shards count;
 * }
 *
 * @param res {
 *     per_shard_relabeler uintptr // pointer to constructed PerShardRelabeler;
 *     error               []byte  // error string if thrown;
 * }
 */
void prompp_prometheus_per_shard_relabeler_ctor(void* args, void* res);

/**
 * @brief Destroy PerShardRelabeler.
 *
 * @param args {
 *     per_shard_relabeler uintptr // pointer of PerShardRelabeler;
 * }
 */
void prompp_prometheus_per_shard_relabeler_dtor(void* args);

/**
 * @brief return size of allocated memory for cache map.
 *
 * @param args {
 *     per_shard_relabeler uintptr // pointer to constructed per shard relabeler;
 * }
 *
 * @param res {
 *     allocated_memory    uint64  // size of allocated memory for label sets;
 * }
 */
void prompp_prometheus_per_shard_relabeler_cache_allocated_memory(void* args, void* res);

/**
 * @brief relabeling incomig hashdex(first stage).
 *
 * @param args {
 *     shards_inner_series     []*InnerSeries     // go slice with InnerSeries;
 *     shards_relabeled_series []*RelabeledSeries // go slice with RelabeledSeries;
 *     options                 RelabelerOptions   // object RelabelerOptions;
 *     per_shard_relabeler     uintptr            // pointer to constructed per shard relabeler;
 *     hashdex                 uintptr            // pointer to filled hashdex;
 *     cache                   uintptr            // pointer to constructed Cache;
 *     input_lss               uintptr            // pointer to constructed input label sets;
 *     target_lss              uintptr            // pointer to constructed target label sets;
 * }
 *
 * @param res {
 *     error                   []byte             // error string if thrown;
 * }
 */
void prompp_prometheus_per_shard_relabeler_input_relabeling(void* args, void* res);

/**
 * @brief Create StaleNaNsState.
 *
 * @param res {
 *     state uintptr // pointer to constructed StaleNaNsState;
 * }
 */
void prompp_prometheus_relabel_stalenans_state_ctor(void* res);

/**
 * @brief Destroy StaleNaNsState.
 *
 * @param args {
 *      state uintptr // pointer to StaleNaNsState;
 * }
 */
void prompp_prometheus_relabel_stalenans_state_dtor(void* args);

/**
 * @brief Reset StaleNaNsState.
 *
 * @param args {
 *      state uintptr // pointer to StaleNaNsState;
 * }
 */
void prompp_prometheus_relabel_stalenans_state_reset(void* args);

/**
 * @brief relabeling incomig hashdex(first stage) with state stalenans.
 *
 * @param args {
 *     shards_inner_series     []*InnerSeries     // go slice with InnerSeries;
 *     shards_relabeled_series []*RelabeledSeries // go slice with RelabeledSeries;
 *     options                 RelabelerOptions   // object RelabelerOptions;
 *     per_shard_relabeler     uintptr            // pointer to constructed per shard relabeler;
 *     hashdex                 uintptr            // pointer to filled hashdex;
 *     cache                   uintptr            // pointer to constructed Cache;
 *     input_lss               uintptr            // pointer to constructed input label sets;
 *     target_lss              uintptr            // pointer to constructed target label sets;
 *     state                   uintptr            // pointer to source state
 *     def_timestamp           int64              // timestamp for metrics and StaleNaNs
 * }
 *
 * @param res {
 *     error                   []byte             // error string if thrown;
 * }
 */
void prompp_prometheus_per_shard_relabeler_input_relabeling_with_stalenans(void* args, void* res);

/**
 * @brief write stale nans from state.
 *
 * @param args {
 *     shards_inner_series     []*InnerSeries     // go slice with InnerSeries;
 *     per_shard_relabeler     uintptr            // pointer to constructed per shard relabeler;
 *     cache                   uintptr            // pointer to constructed Cache;
 *     state                   uintptr            // pointer to source state
 *     stale_ts                int64              // timestamp for StaleNaNs
 * }
 *
 * @param res {
 *     error                   []byte             // error string if thrown;
 * }
 */
void prompp_prometheus_per_shard_relabeler_input_collect_stalenans(void* args, void* res);

/**
 * @brief add relabeled ls to lss, add to result and add to cache update(second stage).
 *
 * @param args {
 *     inner_series           *InnerSeries          // go InnerSeries per shard;
 *     relabeled_series       *RelabeledSeries      // go RelabeledSeries per shard;
 *     relabeler_state_update *RelabelerStateUpdate // pointer to RelabelerStateUpdate;
 *     per_shard_relabeler    uintptr               // pointer to constructed per shard relabeler;
 *     lss                    uintptr               // pointer to constructed label sets;
 * }
 *
 * @param res {
 *     error                  []byte           // error string if thrown
 * }
 */
void prompp_prometheus_per_shard_relabeler_append_relabeler_series(void* args, void* res);

/**
 * @brief add to cache relabled data(third stage).
 *
 * @param args {
 *     relabeler_state_update *RelabelerStateUpdate // pointer to RelabelerStateUpdate;
 *     per_shard_relabeler    uintptr               // pointer to constructed per shard relabeler;
 *     cache                  uintptr               // pointer to constructed Cache;
 *     relabeled_shard_id     uint16                // relabeled shard id;
 * }
 *
 * @param res {
 *     error                  []byte  // error string if thrown;
 * }
 */
void prompp_prometheus_per_shard_relabeler_update_relabeler_state(void* args, void* res);

/**
 * @brief relabeling output series(fourth stage).
 *
 * @param args {
 *     incoming_inner_series     []*InnerSeries     // go slice with incoming InnerSeries;
 *     encoders_inner_series     []*InnerSeries     // go slice with output InnerSeries;
 *     shards_relabeled_series   []*RelabeledSeries // go slice with output RelabeledSeries;
 *     per_shard_relabeler       uintptr            // pointer to constructed per shard relabeler;
 *     lss                       uintptr            // pointer to constructed label sets;
 *     cache                     uintptr            // pointer to constructed Cache;
 * }
 *
 * @param res {
 *     error                   []byte             // error string if thrown;
 * }
 */
void prompp_prometheus_per_shard_relabeler_output_relabeling(void* args, void* res);

/**
 * @brief reset set new number_of_shards and external_labels.
 *
 * @param args {
 *     external_labels     []Label // slice with external lables(pair string);
 *     per_shard_relabeler uintptr // pointer to constructed per shard relabeler;
 *     number_of_shards    uint16  // total shards count;
 * }
 */
void prompp_prometheus_per_shard_relabeler_reset_to(void* args);

//
// Relabeler cache
//

/**
 * @brief Construct a new Cache.
 *
 * @param res {
 *     cache               uintptr // pointer to constructed Cache;
 * }
 */
void prompp_prometheus_cache_ctor(void* res);

/**
 * @brief Destroy Cache.
 *
 * @param args {
 *     cache               uintptr // pointer to constructed Cache;
 * }
 */
void prompp_prometheus_cache_dtor(void* args);

/**
 * @brief return size of allocated memory for caches.
 *
 * @param args {
 *     cache               uintptr // pointer to constructed Cache;
 * }
 *
 * @param res {
 *     allocated_memory    uint64  // size of allocated memory for label sets;
 * }
 */
void prompp_prometheus_cache_allocated_memory(void* args, void* res);

/**
 * @brief reset cache and store lss generation.
 *
 * @param args {
 *     cache               uintptr // pointer to constructed Cache;
 * }
 */
void prompp_prometheus_cache_reset_to(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Construct a new series data DataStorage
 *
 * @param res {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 */
void prompp_series_data_data_storage_ctor(void* res);

/**
 * @brief Resets DataStorage to initial state
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 */
void prompp_series_data_data_storage_reset(void* args);

/**
 * @brief Get min max timestamps in storage
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 *
 * @param res {
 *     interval struct {
 *        min int64
 *        max int64
 *     }
 * }
 *
 */
void prompp_series_data_data_storage_time_interval(void* args, void* res);

/**
 * @brief Queries data storage and serializes result.
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 *
 * @param args {
 *     allocated_memory uint64 // serialized data
 * }
 */
void prompp_series_data_data_storage_allocated_memory(void* args, void* res);

/**
 * @brief Queries data storage and serializes result.
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 *     query DataStorageQuery // query
 * }
 *
 * @param args {
 *     serializedData []byte // serialized data
 * }
 */
void prompp_series_data_data_storage_query(void* args, void* res);

/**
 * @brief series data DataStorage destructor.
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 */
void prompp_series_data_data_storage_dtor(void* args);

/**
 * @brief Construct a new ChunkRecoder object
 *
 * @param args {
 *     dataStorage   uintptr  // pointer to constructed data storage
 *     time_interval struct { interval is semi-open [min, max)
 *        min int64
 *        max int64
 *     }
 * }
 * @param res {
 *     chunk_recoder uintptr // pointer to chunk recoder
 * }
 */
void prompp_series_data_chunk_recoder_ctor(void* args, void* res);

/**
 * @brief Get chunk encoded in prometheus format
 *
 * @param args {
 *     chunk_recoder uintptr // pointer to chunk recoder
 * }
 * @param res {
 *     interval struct {
 *        min int64
 *        max int64
 *     }
 *     series_id     uint32
 *     samples_count uint8
 *     has_more_data bool
 *     data          []byte // SliceView to recoded chunk data
 * }
 */
void prompp_series_data_chunk_recoder_recode_next_chunk(void* args, void* res);

/**
 * @brief Destruct ChunkRecoder object
 *
 * @param args {
 *     chunk_recoder  uintptr  // pointer to chunk recoder
 * }
 */
void prompp_series_data_chunk_recoder_dtor(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

void prompp_series_data_decode_iterator_next(void* args, void* res);
void prompp_series_data_decode_iterator_sample(void* args, void* res);
void prompp_series_data_decode_iterator_dtor(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief series data Deserializer constructor.
 *
 * @param args {
 *     serializedChunks []byte // serialized chunks data.
 * }
 *
 * @param res {
 *     deserializer uintptr // pointer to constructed deserializer.
 * }
 */
void prompp_series_data_deserializer_ctor(void* args, void* res);

/**
 * @brief creates decode iterator for chunk.
 *
 * @param args {
 *     deserializer  uintptr // deserializer.
       chunkMetadata []byte  // chunk metadata.
 * }
 *
 * @param res {
 *     decodeIterator uintptr // pointer to constructed encoder
 * }
 */
void prompp_series_data_deserializer_create_decode_iterator(void* args, void* res);

/**
 * @brief series data Deserializer destructor.
 *
 * @param args {
 *     deserializer uintptr // pointer to constructed deserializer
 * }
 */
void prompp_series_data_deserializer_dtor(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief series data Encoder constructor.
 *
 * @param args {
 *     data_storage uintptr // pointer to constructed data storage
 * }
 *
 * @param res {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
void prompp_series_data_encoder_ctor(void* args, void* res);

/**
 * @brief adds single series to data storage
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 *     seriesID uint32 // series id
 *     timestamp int64 // timestamp
 *     value float64   // value
 * }
 */
void prompp_series_data_encoder_encode(void* args);

/**
 * @brief adds slice of inner series to data storage
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 *     innerSeriesSlice []*InnerSeries // pointer to inner series slice.
 * }
 */
void prompp_series_data_encoder_encode_inner_series_slice(void* args);

/**
 * @brief merge outdated chunks
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
void prompp_series_data_encoder_merge_out_of_order_chunks(void* args);

/**
 * @brief series data Encoder destructor.
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
void prompp_series_data_encoder_dtor(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
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
 *     segment               []byte      // segment content
 *     decoder               uintptr     // pointer to constructed output decoder
 *     lower_limit_timestamp int64       // lower limit timestamp
 * }
 *
 * @param res {
 *     max_timestamp         int64       // max timestamp in slice RefSample
 *     outdated_sample_count uint64      // count of dropped samples on outdated
 *     dropped_sample_count  uint64      // count of dropped samples on relabeling rules
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
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Destroy hashdex
 *
 * @param args {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
void prompp_wal_hashdex_dtor(void* args);

/**
 * @brief Construct a new WAL Hashdex
 *
 * @param args { // limits for incoming data
 *     max_label_name_length          uint32
 *     max_label_value_length         uint32
 *     max_label_names_per_timeseries uint32
 *     max_timeseries_count           uint64
 *     max_pb_size_in_bytes           uint64
 * }
 * @param res {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
void prompp_wal_protobuf_hashdex_ctor(void* args, void* res);

/**
 * @brief Fill hashdex from compressed via snappy protobuf
 *
 * Hashdex only indexing protobuf and doesn't copy all data.
 * Caller should preserve original protobuf content at the same
 * memory address to use hashdex in next call.
 *
 * @param args {
 *     hashdex             uintptr // pointer to constructed hashdex
 *     compressed_protobuf []byte  // compressed via snappy RemoteWrite protobuf content
 * }
 * @param res {
 *     // this data is a view over protobuf memory and shouldn't be destroyed explicitely
 *     cluster string // value of label cluster from first sample
 *     replica string // value of label __replica__ from first sample
 *     error   []byte // error string if thrown
 * }
 */
void prompp_wal_protobuf_hashdex_snappy_presharding(void* args, void* res);

/**
 * @brief Get parsed metadata
 *
 * @param args {
 *     hashdex uintptr
 * }
 * @param res {
 *     metadata []struct {
 *        metric_name string
 *        text string
 *        type uint32
 *     }
 * }
 */
void prompp_wal_protobuf_hashdex_get_metadata(void* args, void* res);

/**
 * @brief Construct a new WAL GoModelHashdex
 *
 * @param args { // limits for incoming data
 *     max_label_name_length          uint32
 *     max_label_value_length         uint32
 *     max_label_names_per_timeseries uint32
 *     max_timeseries_count           uint64
 * }
 * @param res {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
void prompp_wal_go_model_hashdex_ctor(void* args, void* res);

/**
 * @brief Fill hashdex from Go memory
 *
 * Hashdex only indexing go memory (model.TimeSeries) and doesn't copy all data.
 * Caller should preserve original protobuf content at the same
 * memory address to use hashdex in next call.
 *
 * @param args {
 *     hashdex  uintptr // pointer to constructed hashdex
 *     data     []model.TimeSeries  // Go content
 * }
 * @param res {
 *     // this data is a view over go memory and shouldn't be destroyed explicitely
 *     cluster string // value of label cluster from first sample
 *     replica string // value of label __replica__ from first sample
 *     error   []byte // error string if thrown
 * }
 */
void prompp_wal_go_model_hashdex_presharding(void* args, void* res);

/**
 * @brief Construct a new PromPP::WAL::hashdex::Scraper based on Prometheus parser
 *
 * @param res {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
void prompp_wal_prometheus_scraper_hashdex_ctor(void* res);

/**
 * @brief Parse scraped buffer
 *
 * @param args {
 *     hashdex           uintptr
 *     buffer            string // buffer will be modified by parser
 *     default_timestamp int64
 * }
 * @param res {
 *     error uint32 // value of PromPP::WAL::hashdex::Scraper::Error
 * }
 */
void prompp_wal_prometheus_scraper_hashdex_parse(void* args, void* res);

/**
 * @brief Get scraped metadata
 *
 * @param args {
 *     hashdex uintptr
 * }
 * @param res {
 *     metadata []struct {
 *        metric_name string
 *        text string
 *        type uint32
 *     }
 * }
 */
void prompp_wal_prometheus_scraper_hashdex_get_metadata(void* args, void* res);

/**
 * @brief Construct a new PromPP::WAL::hashdex::Scraper based on OpenMetrics parser
 *
 * @param res {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
void prompp_wal_open_metrics_scraper_hashdex_ctor(void* res);

/**
 * @brief Parse scraped buffer
 *
 * @param args {
 *     hashdex           uintptr
 *     buffer            string // buffer will be modified by parser
 *     default_timestamp int64
 * }
 * @param res {
 *     error uint32 // value of PromPP::WAL::hashdex::Scraper::Error
 * }
 */
void prompp_wal_open_metrics_scraper_hashdex_parse(void* args, void* res);

/**
 * @brief Get scraped metadata
 *
 * @param args {
 *     hashdex uintptr
 * }
 * @param res {
 *     metadata []struct {
 *        metric_name string
 *        text string
 *        type uint32
 *     }
 * }
 */
void prompp_wal_open_metrics_scraper_hashdex_get_metadata(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief return determined flavor
 *
 * @param res {
 *   flavor string
 * }
 */
void prompp_get_flavor(void* res);

#ifdef __cplusplus
}
#endif
