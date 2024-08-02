#pragma once

#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Construct index writer
 *
 * @param args {
 *     lss         uintptr     // pointer to constructed head
 *     chunks_meta *[][]struct{ // index in first slice is series id
 *         min_t int64
 *         max_t int64
 *         size  uint32
 *     }
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
 *     data *[]byte // only c allocated memory can be re-used
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
 *     data *[]byte // only c allocated memory can be re-used
 * }
 */
void prompp_index_writer_write_symbols(void* args, void* res);

/**
 * @brief Write next series batch
 *
 * @param args {
 *     writer     uintptr
 *     batch_size uint32
 * }
 * @param res {
 *     has_more_data bool   // true if we should repeat this call
 *     data          *[]byte // only c allocated memory can be re-used
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
 *     data *[]byte // only c allocated memory can be re-used
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
 *     has_more_data bool   // true if we should repeat this call
 *     data          *[]byte // only c allocated memory can be re-used
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
 *     data *[]byte // only c allocated memory can be re-used
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
