#include "wal/wal.h"

#include "wal_basic_encoder.h"

#include <cassert>
#include <new>

// for brevity, disable clang-tidy error for 'using namespace %s'.
// NOLINTNEXTLINE(google-build-using-namespace)
using namespace PromPP::WAL;

#define BE(basic_encoder) static_cast<BasicEncoder<>*>(basic_encoder)

extern "C" {
/**
 * Factory for creating /ref BasicEncoder with default LabelSetsTable.
 */
basic_encoder_ptr OKDB_WAL_PREFIXED_NAME(okdb_wal_basic_encoder_create)() {
  return new (std::nothrow) BasicEncoder<>();
}

void OKDB_WAL_PREFIXED_NAME(okdb_wal_basic_encoder_destroy)(basic_encoder_ptr encoder) {
  delete BE(encoder);
}

}  // extern "C"
