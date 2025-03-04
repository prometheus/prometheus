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
