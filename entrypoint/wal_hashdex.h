#ifdef __cplusplus
extern "C" {
#endif

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
void prompp_wal_hashdex_ctor(void* args, void* res);

/**
 * @brief Destroy hashdex
 *
 * @param args {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
void prompp_wal_hashdex_dtor(void* args);

/**
 * @brief Fill hashdex from protobuf
 *
 * Hashdex only indexing protobuf and doesn't copy all data.
 * Caller should preserve original protobuf content at the same
 * memory address to use hashdex in next call.
 *
 * @param args {
 *     hashdex  uintptr // pointer to constructed hashdex
 *     protobuf []byte  // RemoteWrite protobuf content
 * }
 * @param res {
 *     // this data is a view over protobuf memory and shouldn't be destroyed explicitely
 *     cluster string // value of label cluster from first sample
 *     replica string // value of label __replica__ from first sample
 *     error   []byte // error string if thrown
 * }
 */
void prompp_wal_hashdex_presharding(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
