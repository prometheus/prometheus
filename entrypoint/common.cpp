#include "common.h"

#if __has_include(<jemalloc/jemalloc.h>)
#define JEMALLOC_AVAILABLE 1
#endif

#if JEMALLOC_AVAILABLE
#include <jemalloc/jemalloc.h>
#else
#include <malloc.h>
#endif

#include "primitives/go_slice.h"

extern "C" void prompp_free_bytes(void* args) {
  using args_t = PromPP::Primitives::Go::Slice<char>;

  args_t* in = reinterpret_cast<args_t*>(args);
  in->free();
}

extern "C" void prompp_mem_info(void* res) {
  using res_t = struct {
    int64_t in_use;
  };

  res_t* out = new (res) res_t();

#if JEMALLOC_AVAILABLE
  uint64_t epoch = 1;
  size_t sz = sizeof(epoch);
  mallctl("epoch", &epoch, &sz, &epoch, sz);
  size_t size;
  size_t size_len = sizeof(size);
  mallctl("stats.allocated", &size, &size_len, NULL, 0);
  out->in_use = size;
#else
  struct mallinfo2 mi;
  mi = mallinfo2();
  out->in_use = mi.uordblks;
#endif
}
