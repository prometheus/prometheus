#include "wal/wal.h"

#include "wal_basic_decoder.h"

#include <cassert>
#include <new>

// for brevity, disable clang-tidy error for 'using namespace %s'.
// NOLINTNEXTLINE(google-build-using-namespace)
using namespace PromPP::WAL;

#define BD(basic_decoder) static_cast<BasicDecoder<>*>(basic_decoder)

// NOTE: All these APIs must be enclosed by OKDB_WAL_PREFIXED_NAME()
// macro, otherwise the main multiarch lib will fail on link stage!!

extern "C" {
/**
 * Factory for creating /ref BasicDecoder with default LabelSetsTable.
 */
basic_decoder_ptr OKDB_WAL_PREFIXED_NAME(okdb_wal_basic_decoder_create)() {
  return new (std::nothrow) BasicDecoder<>();
}

// Getters
uint32_t OKDB_WAL_PREFIXED_NAME(okdb_wal_basic_decoder_get_series_count)(basic_decoder_ptr reader) {
  assert(reader);
  return BD(reader)->series();
}

uint32_t OKDB_WAL_PREFIXED_NAME(okdb_wal_basic_decoder_get_samples_count)(basic_decoder_ptr reader) {
  assert(reader);
  return BD(reader)->samples();
}

// dtor
void OKDB_WAL_PREFIXED_NAME(okdb_wal_basic_decoder_destroy)(basic_decoder_ptr reader) {
  static_assert(std::is_same_v<decltype(reader), void*>);
  delete BD(reader);
}

}  // extern "C"
