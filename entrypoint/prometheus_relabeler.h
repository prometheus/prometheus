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
 *     generation             uint32                // current generation;
 * }
 */
void prompp_prometheus_relabeler_state_update_ctor(void* res);

/**
 * @brief Destroy vector in RelabelerStateUpdate.
 *
 * @param args {
 *      relabeledSeries *RelabeledSeries // pointer to RelabeledSeries;
 * }
 */
void prompp_prometheus_relabeler_state_update_dtor(void* args);

/**
 * @brief Destroy StaleNaNsState.
 *
 * @param args {
 *      sourceState uintptr // pointer to StaleNaNsState;
 * }
 */
void prompp_prometheus_stalenans_state_dtor(void* args);

//
// PerShardRelabeler
//

/**
 * @brief Construct a new PerShardRelabeler.
 *
 * @param args {
 *     external_labels     []Label // slice with external labels;
 *     stateless_relabeler uintptr // pointer to constructed stateless relabeler;
 *     generation          uint32  // generation of lss;
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
 *     lss                     uintptr            // pointer to constructed label sets;
 * }
 *
 * @param res {
 *     error                   []byte             // error string if thrown;
 * }
 */
void prompp_prometheus_per_shard_relabeler_input_relabeling(void* args, void* res);

/**
 * @brief relabeling incomig hashdex(first stage) with state stalenans.
 *
 * @param args {
 *     shards_inner_series     []*InnerSeries     // go slice with InnerSeries;
 *     shards_relabeled_series []*RelabeledSeries // go slice with RelabeledSeries;
 *     options                 RelabelerOptions   // object RelabelerOptions;
 *     per_shard_relabeler     uintptr            // pointer to constructed per shard relabeler;
 *     hashdex                 uintptr            // pointer to filled hashdex;
 *     lss                     uintptr            // pointer to constructed label sets;
 *     source_state            uintptr            // pointer to source state (null on first call)
 *     stale_ts                int64              // timestamp for StaleNaNs
 * }
 *
 * @param res {
 *     source_state            uintptr            // pointer to internal source state
 *     error                   []byte             // error string if thrown;
 * }
 */
void prompp_prometheus_per_shard_relabeler_input_relabeling_with_stalenans(void* args, void* res);

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
 *     generation                uint32             // current encoders generation;
 * }
 *
 * @param res {
 *     error                   []byte             // error string if thrown;
 * }
 */
void prompp_prometheus_per_shard_relabeler_output_relabeling(void* args, void* res);

/**
 * @brief reset cache and store lss generation.
 *
 * @param args {
 *     external_labels     []Label // slice with external lables(pair string);
 *     per_shard_relabeler uintptr // pointer to constructed per shard relabeler;
 *     generation          uint32  // generation of lss;
 *     number_of_shards    uint16  // total shards count;
 * }
 */
void prompp_prometheus_per_shard_relabeler_reset_to(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
