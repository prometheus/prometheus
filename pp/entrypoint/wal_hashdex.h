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
