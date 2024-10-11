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
 *          time_interval {
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
