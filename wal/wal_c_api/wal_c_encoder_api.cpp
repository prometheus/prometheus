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
#include "wal_c_encoder.h"

// manually declare bunch of functions
#undef OKDB_WAL_FUNCTION_NAME_PREFIX
#define OKDB_WAL_FUNCTION_NAME_PREFIX x86_nehalem_
#include "wal_c_encoder.h"

// manually declare bunch of functions
#undef OKDB_WAL_FUNCTION_NAME_PREFIX
#define OKDB_WAL_FUNCTION_NAME_PREFIX x86_haswell_
#include "wal_c_encoder.h"
#undef OKDB_WAL_FUNCTION_NAME_PREFIX
#define TOTAL_FLAVOURS 3
static_assert(TOTAL_FLAVOURS == arch_count, "Update the #includes for new arch flavours!");
#else

#define OKDB_WAL_FUNCTION_NAME_PREFIX aarch64_generic_
#include "wal_c_encoder.h"
#define OKDB_WAL_FUNCTION_NAME_PREFIX aarch64_crc_
#include "wal_c_encoder.h"

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
    x86_generic_okdb_wal_encoder_api_vtbl,
    x86_nehalem_okdb_wal_encoder_api_vtbl,
    x86_haswell_okdb_wal_encoder_api_vtbl
#else
    arm_okdb_wal_encoder_api_vtbl,
    arm_crc_okdb_wal_encoder_api_vtbl
#endif
    >
    encoder_vtbl;

#define SETUP_VTBL(vtbl, type) (vtbl).emplace(type{})

void setup_arch_functions(size_t index) {
  switch (index) {
#ifndef __arm__
    default:
    case generic:
      encoder_vtbl.emplace<x86_generic_okdb_wal_encoder_api_vtbl>();
      break;
    case nehalem:
      encoder_vtbl.emplace<x86_nehalem_okdb_wal_encoder_api_vtbl>();
      break;
    case haswell:
      encoder_vtbl.emplace<x86_haswell_okdb_wal_encoder_api_vtbl>();
      break;
#else
    default:
    case generic:
      encoder_vtbl.emplace<arm_okdb_wal_encoder_api_vtbl>();
      break;
    case crc32:
      encoder_vtbl.emplace<arm_crc_okdb_wal_encoder_api_vtbl>();
      break;
#endif
  }
}

}  // namespace

extern "C" {
#undef OKDB_WAL_PREFIXED_NAME
#define OKDB_WAL_PREFIXED_NAME(name) name

// encoder and types routed C API

// okdb_wal_c_redundant_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_redundant_destroy(c_redundant* c_rt) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_redundant_destroy">(c_rt); }, encoder_vtbl);
}

// Hashdex
// okdb_wal_c_hashdex_ctor - constructor, C wrapper C++, init C++ class Hashdex.
c_hashdex okdb_wal_c_hashdex_ctor() {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_hashdex_ctor">(); }, encoder_vtbl);
}

// okdb_wal_c_hashdex_presharding - C wrapper C++, calls C++ class Hashdex methods.
void okdb_wal_c_hashdex_presharding(c_hashdex c_hx, c_slice proto_data) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_hashdex_presharding">(c_hx, proto_data); }, encoder_vtbl);
}

// okdb_wal_c_hashdex_dtor - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_hashdex_dtor(c_hashdex c_hx) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_hashdex_dtor">(c_hx); }, encoder_vtbl);
}

// Encoder
// okdb_wal_c_encoder_ctor - constructor, C wrapper C++, init C++ class Encoder.
c_encoder okdb_wal_c_encoder_ctor(uint16_t shard_id, uint16_t number_of_shards) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_encoder_ctor">(shard_id, number_of_shards); }, encoder_vtbl);
}

// okdb_wal_c_encoder_encode - C wrapper C++, calls C++ class Encoder methods.
void okdb_wal_c_encoder_encode(c_encoder c_enc, c_hashdex c_hx, c_slice_with_stream_buffer* c_seg, c_redundant* c_rt) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_encoder_encode">(c_enc, c_hx, c_seg, c_rt); }, encoder_vtbl);
}

// okdb_wal_c_encoder_snapshot - C wrapper C++, calls C++ class Encoder methods.
void okdb_wal_c_encoder_snapshot(c_encoder c_enc, c_slice c_rts, c_slice_with_stream_buffer* c_snap) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_encoder_snapshot">(c_enc, c_rts, c_snap); }, encoder_vtbl);
}

// okdb_wal_c_encoder_dtor - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_encoder_dtor(c_encoder c_enc) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_encoder_dtor">(c_enc); }, encoder_vtbl);
}

// Entry point

/// \brief Entry point for WAL C API. Call this function before
///        using any another.
/// \details
//           Initializes the arch-optimized version of C API, detecting the
//           supported architecture features at run-time.
/// \returns arch flavour ID.

int okdb_wal_initialize_encoder() {
  auto cur_arch_flavour = determine_arch_flavour();

  setup_arch_functions(cur_arch_flavour);
  return cur_arch_flavour;
}

}  // extern "C"
