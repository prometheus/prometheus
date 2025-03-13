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
  using Slice = PromPP::Primitives::Go::Slice<char>;

  static_cast<Slice*>(args)->~Slice();
}

extern "C" void je_jemalloc_constructor(void);

extern "C" void prompp_jemalloc_init() {
#if JEMALLOC_AVAILABLE
  je_jemalloc_constructor();
#endif
}

extern "C" void prompp_mem_info(void* res) {
  struct Result {
    int64_t in_use;
    int64_t allocated;
  };

  const auto out = static_cast<Result*>(res);

#if JEMALLOC_AVAILABLE
  uint64_t epoch = 1;
  size_t sz = sizeof(epoch);
  mallctl("epoch", &epoch, &sz, &epoch, sz);
  size_t size;
  size_t size_len = sizeof(size);
  mallctl("stats.active", &size, &size_len, NULL, 0);
  out->in_use = size;
  mallctl("stats.allocated", &size, &size_len, NULL, 0);
  out->allocated = size;
#else
  out->in_use = mallinfo2().uordblks;
#endif
}

extern "C" void prompp_dump_memory_profile([[maybe_unused]] void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::String filename;
  };
  struct Result {
    int error;
  };

  const auto out = static_cast<Result*>(res);

#if JEMALLOC_AVAILABLE
  auto in = static_cast<Arguments*>(args);
  std::string filename_c_string(in->filename.data(), in->filename.size());
  const char* filename = filename_c_string.c_str();

  out->error = mallctl("prof.dump", nullptr, nullptr, &filename, sizeof(const char*));
#else
  out->error = ENODATA;
#endif
}
