#ifndef LRU_CACHE_LRU_CACHE_H_
#define LRU_CACHE_LRU_CACHE_H_

#include "dynamic_lru_cache.h"
#include "node_lru_cache.h"
#include "static_lru_cache.h"

namespace lru_cache
{

// Create simple cache
template <typename Key, typename Value>
NodeLruCache<Key, Value>
make_cache(
    size_t max_size) {
  return {max_size};
}

// Memoize a single-argument function.
template <typename ValueProvider>
NodeLruCache<internal::single_arg_t<ValueProvider>,
             internal::return_t<ValueProvider>, ValueProvider>
memoize_function(
    size_t max_size, ValueProvider v) {
  return {max_size, v};
}
}

#endif  // LRU_CACHE_LRU_CACHE_H_
