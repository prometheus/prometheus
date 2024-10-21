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
 *     min_t         int64
 *     max_t         int64
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
