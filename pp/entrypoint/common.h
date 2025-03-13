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
