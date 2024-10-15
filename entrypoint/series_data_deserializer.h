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
