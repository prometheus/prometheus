#ifdef __cplusplus
extern "C" {
#endif

//
// LSS EncodingBimap
//

/**
 * @brief Construct a new Primitives label sets.
 *
 * @param res {
 *     lss uintptr // pointer to constructed label sets;
 * }
 */
void prompp_primitives_lss_ctor(void* res);

/**
 * @brief Destroy Primitives label sets.
 *
 * @param args {
 *     lss uintptr // pointer of label sets;
 * }
 */
void prompp_primitives_lss_dtor(void* args);

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr             // pointer to constructed label sets;
 * }
 *
 * @param res {
 *     allocated_memory uint64 // size of allocated memory for label sets;
 * }
 */
void prompp_primitives_lss_allocated_memory(void* args, void* res);

//
// LSS OrderedEncodingBimap
//

/**
 * @brief Construct a new Primitives ordered label sets.
 *
 * @param res {
 *     orderedLss uintptr // pointer to constructed ordered label sets;
 * }
 */
void prompp_primitives_ordered_lss_ctor(void* res);

/**
 * @brief Destroy Primitives ordered label sets.
 *
 * @param args {
 *     orderedLss uintptr // pointer of ordered label sets;
 * }
 */
void prompp_primitives_ordered_lss_dtor(void* args);

/**
 * @brief return size of allocated memory for ordered label sets.
 *
 * @param args {
 *     orderedLss uintptr     // pointer to constructed ordered label sets;
 * }
 *
 * @param res {
 *     allocatedMemory uint64 // size of allocated memory for ordered label sets;
 * }
 */
void prompp_primitives_ordered_lss_allocated_memory(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
