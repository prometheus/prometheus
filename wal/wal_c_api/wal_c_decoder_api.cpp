#include <string>
#include <string_view>
#include <variant>

#include "arch_detector/arch_detector.h"

// supported arch flavours
enum arch_flavour {
  generic,
#ifndef __arm__
  nehalem,
  haswell,
#else
  crc32,
#endif
  arch_count,
};

#ifndef __arm__
#undef OKDB_WAL_FUNCTION_NAME_PREFIX
#define OKDB_WAL_FUNCTION_NAME_PREFIX x86_generic_
#include "wal_c_decoder.h"

// manually declare bunch of functions
#undef OKDB_WAL_FUNCTION_NAME_PREFIX
#define OKDB_WAL_FUNCTION_NAME_PREFIX x86_nehalem_
#include "wal_c_decoder.h"

// manually declare bunch of functions
#undef OKDB_WAL_FUNCTION_NAME_PREFIX
#define OKDB_WAL_FUNCTION_NAME_PREFIX x86_haswell_
#include "wal_c_decoder.h"
#undef OKDB_WAL_FUNCTION_NAME_PREFIX
#define TOTAL_FLAVOURS 3
static_assert(TOTAL_FLAVOURS == arch_count, "Update the #includes for new arch flavours!");
#else

#define OKDB_WAL_FUNCTION_NAME_PREFIX aarch64_generic_
#include "wal_c_decoder.h"
#define OKDB_WAL_FUNCTION_NAME_PREFIX aarch64_crc_
#include "wal_c_decoder.h"

#endif

namespace {
arch_flavour determine_arch_flavour() {
  auto flags = arch_detector::detect_supported_architectures();
  if (flags & (arch_detector::instruction_set::BMI1 | arch_detector::instruction_set::AVX2)) {
    return haswell;
  }
  if (flags & (arch_detector::instruction_set::SSE42)) {
    return nehalem;
  }
  return generic;
}

std::variant<
#ifndef __arm__
    x86_generic_okdb_wal_decoder_api_vtbl,
    x86_nehalem_okdb_wal_decoder_api_vtbl,
    x86_haswell_okdb_wal_decoder_api_vtbl
#else
    arm_okdb_wal_decoder_api_vtbl,
    arm_crc_okdb_wal_decoder_api_vtbl
#endif
    >
    decoder_vtbl;

#define SETUP_VTBL(vtbl, type) (vtbl).emplace(type{})

void setup_arch_functions(size_t index) {
  switch (index) {
#ifndef __arm__
    default:
    case generic:
      decoder_vtbl.emplace<x86_generic_okdb_wal_decoder_api_vtbl>();
      break;
    case nehalem:
      decoder_vtbl.emplace<x86_nehalem_okdb_wal_decoder_api_vtbl>();
      break;
    case haswell:
      decoder_vtbl.emplace<x86_haswell_okdb_wal_decoder_api_vtbl>();
      break;
#else
    default:
    case generic:
      decoder_vtbl.emplace<arm_okdb_wal_decoder_api_vtbl>();
      break;
    case crc32:
      decoder_vtbl.emplace<arm_crc_okdb_wal_decoder_api_vtbl>();
      break;
#endif
  }
}

}  // namespace

extern "C" {
#undef OKDB_WAL_PREFIXED_NAME
#define OKDB_WAL_PREFIXED_NAME(name) name

// decoder and types routed C API

// Decoder
// okdb_wal_c_decoder_ctor - constructor, C wrapper C++, init C++ class Decoder.
c_decoder okdb_wal_c_decoder_ctor() {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_decoder_ctor">(); }, decoder_vtbl);
}

// okdb_wal_c_decoder_decode - C wrapper C++, calls C++ class Decoder methods.
uint32_t okdb_wal_c_decoder_decode(c_decoder c_dec, c_slice c_seg, c_slice_with_string_buffer* c_protobuf) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_decoder_decode">(c_dec, c_seg, c_protobuf); }, decoder_vtbl);
}

// okdb_wal_c_decoder_decode_dry - C wrapper C++, calls C++ class Decoder methods.
uint32_t okdb_wal_c_decoder_decode_dry(c_decoder c_dec, c_slice c_seg) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_decoder_decode_dry">(c_dec, c_seg); }, decoder_vtbl);
}

// okdb_wal_c_decoder_snapshot - C wrapper C++, calls C++ class Decoder methods.
void okdb_wal_c_decoder_snapshot(c_decoder c_dec, c_slice c_snap) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_decoder_snapshot">(c_dec, c_snap); }, decoder_vtbl);
}

// okdb_wal_c_decoder_dtor - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_decoder_dtor(c_decoder c_dec) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_decoder_dtor">(c_dec); }, decoder_vtbl);
}

// Entry point

/// \brief Entry point for WAL C API. Call this function before
///        using any another.
/// \details
//           Initializes the arch-optimized version of C API, detecting the
//           supported architecture features at run-time.
/// \returns arch flavour ID.

int okdb_wal_initialize_decoder() {
  auto cur_arch_flavour = determine_arch_flavour();

  setup_arch_functions(cur_arch_flavour);
  return cur_arch_flavour;
}

}  // extern "C"
