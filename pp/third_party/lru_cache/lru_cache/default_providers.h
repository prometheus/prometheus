#ifndef LRU_CACHE_DEFAULT_PROVIDERS_H_
#define LRU_CACHE_DEFAULT_PROVIDERS_H_

#include "exception.h"
#include "traits_util.h"

namespace lru_cache::internal {

// To handle value creation manually, throw when a key is missing.
template <typename Key, typename Value>
Value throwing_value_producer(const Key& key) {
  throw KeyNotFound(key);
}

// If there is nothing to do when an entry is dropped, pass this as parameter.
template <typename Key, typename Value>
void no_op_dropped_entry_callback(Key, Value) {}

// Same with argument deduction from a function-like type.
template <typename F>
void no_op_dropped_entry_callback_deduced(internal::single_arg_t<F> key, internal::return_t<F> value) {}

}  // namespace lru_cache::internal
#endif  // LRU_CACHE_DEFAULT_PROVIDERS_H_
