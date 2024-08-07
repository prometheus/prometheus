#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Construct a new Primitives label sets.
 *
 * @param args {
 *     lss_type uint32 // type of lss;
 * }
 *
 * @param res {
 *     lss uintptr     // pointer to constructed label sets;
 * }
 */
void prompp_primitives_lss_ctor(void* args, void* res);

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

/**
 * @brief insert label set into lss
 *
 * @param args {
 *     lss uintptr              // pointer to constructed lss;
 *     label_set model.LabelSet // label set
 * }
 *
 * @param res {
 *     ls_id uint32 // inserted (or found) label set id
 * }
 */
void prompp_primitives_lss_find_or_emplace(void* args, void* res);

/**
 * @brief query series from lss
 *
 * @param args {
 *     lss uintptr                         // pointer to constructed queryable lss;
 *     label_matchers []model.LabelMatcher // label matchers
 * }
 *
 * @param res {
 *     status uint32    // query status
 *     matches []uint32 // matched series ids
 * }
 */
void prompp_primitives_lss_query(void* args, void* res);

/**
 * @brief get label sets by series id
 *
 * @param args {
 *     lss uintptr    // pointer to constructed lss;
 *     ls_id []uint32 // series ids
 * }
 *
 * @param res {
 *     label_sets [][]struct {key, value String} // label sets
 * }
 */
void prompp_primitives_lss_get_label_sets(void* args, void* res);

/**
 * @brief free label sets returned by prompp_primitives_lss_get_label_sets
 *
 * @param args {
 *     label_sets [][]struct {key, value String} // label set
 * }
 */
void prompp_primitives_lss_free_label_sets(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
