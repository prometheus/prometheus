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

#ifdef __cplusplus
}
#endif
