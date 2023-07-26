/// \file wal/wal_c_api.cpp
///       Contains multiarch API wrappers for
///       \ref BasicDecoder<>, \ref BasicEncoder<> classes.
/// \note Use the \ref okdb_wal_initialize() before
///       any function from this module for multiarch initialization.
#include <string>
#include <string_view>
#include <variant>

#include "arch_detector/arch_detector.h"

// supported arch flavours
enum arch_flavour {
  generic,
#if ARCH_DETECTOR_BUILD_FOR_X86_64
  nehalem,
  haswell,
#elif ARCH_DETECTOR_BUILD_FOR_ARM64
  crc32,
#endif
  arch_count,
};

//
// Generate prefixed C API declarations.

#if ARCH_DETECTOR_BUILD_FOR_X86_64

#define OKDB_WAL_FUNCTION_NAME_PREFIX x86_generic_
#include "wal_c_api/wal_c_decoder.h"
#include "wal_c_api/wal_c_encoder.h"

// manually declare bunch of functions
#undef OKDB_WAL_FUNCTION_NAME_PREFIX
#define OKDB_WAL_FUNCTION_NAME_PREFIX x86_nehalem_
#include "wal_c_api/wal_c_decoder.h"
#include "wal_c_api/wal_c_encoder.h"

#undef OKDB_WAL_FUNCTION_NAME_PREFIX
#define OKDB_WAL_FUNCTION_NAME_PREFIX x86_haswell_
#include "wal_c_api/wal_c_decoder.h"
#include "wal_c_api/wal_c_encoder.h"
#undef OKDB_WAL_FUNCTION_NAME_PREFIX
#define TOTAL_FLAVOURS 3
static_assert(TOTAL_FLAVOURS == arch_count, "Update the #includes for new x86_64 arch flavours!");
#elif ARCH_DETECTOR_BUILD_FOR_ARM64  // ^----- x86_64 / aarch64 ------v

#define OKDB_WAL_FUNCTION_NAME_PREFIX aarch64_generic_
#include "wal_c_api/wal_c_decoder.h"
#include "wal_c_api/wal_c_encoder.h"
#undef OKDB_WAL_FUNCTION_NAME_PREFIX
#define OKDB_WAL_FUNCTION_NAME_PREFIX aarch64_crc_
#include "wal_c_api/wal_c_decoder.h"
#include "wal_c_api/wal_c_encoder.h"
#undef OKDB_WAL_FUNCTION_NAME_PREFIX

#define TOTAL_FLAVOURS 2
static_assert(TOTAL_FLAVOURS == arch_count, "Update the #includes for new ARM64 arch flavours!");
#endif

// internal namespace for determining arch instruction set and initializing vtables
namespace {
arch_flavour determine_arch_flavour() {
  auto arch_feature_flags = arch_detector::detect_supported_architectures();

#if ARCH_DETECTOR_BUILD_FOR_X86_64
  constexpr auto required_feature_flags_for_haswell = (arch_detector::instruction_set::BMI1 | arch_detector::instruction_set::AVX2);

  bool cpu_is_nehalem_or_better = arch_feature_flags & (arch_detector::instruction_set::SSE42);
  bool cpu_is_haswell_or_better = cpu_is_nehalem_or_better && ((arch_feature_flags & required_feature_flags_for_haswell) == required_feature_flags_for_haswell);

  return cpu_is_haswell_or_better ? haswell : (cpu_is_nehalem_or_better ? nehalem : generic);

#elif ARCH_DETECTOR_BUILD_FOR_ARM64  // ^----- x86_64 / aarch64 ------v
  if (arch_feature_flags & (arch_detector::instruction_set::CRC32)) {
    return crc32;
  }
#endif
  return generic;
}

/// \brief Vtable with arch flavours for \ref BasicDecoder<> wrapped API.
std::variant<
#if ARCH_DETECTOR_BUILD_FOR_X86_64
    x86_generic_okdb_wal_decoder_api_vtbl,
    x86_nehalem_okdb_wal_decoder_api_vtbl,
    x86_haswell_okdb_wal_decoder_api_vtbl
#elif ARCH_DETECTOR_BUILD_FOR_ARM64
    aarch64_generic_okdb_wal_decoder_api_vtbl,
    aarch64_crc_okdb_wal_decoder_api_vtbl
#endif
    >
    decoder_vtbl;

std::variant<
#if ARCH_DETECTOR_BUILD_FOR_X86_64
    x86_generic_okdb_wal_encoder_api_vtbl,
    x86_nehalem_okdb_wal_encoder_api_vtbl,
    x86_haswell_okdb_wal_encoder_api_vtbl
#elif ARCH_DETECTOR_BUILD_FOR_ARM64
    aarch64_generic_okdb_wal_encoder_api_vtbl,
    aarch64_crc_okdb_wal_encoder_api_vtbl
#endif
    >
    encoder_vtbl;

#define SETUP_VTBL(vtbl, type) vtbl.emplace<type>()

void setup_arch_functions(size_t index) {
  switch (index) {
#if ARCH_DETECTOR_BUILD_FOR_X86_64
    default:
    case generic:
      SETUP_VTBL(decoder_vtbl, x86_generic_okdb_wal_decoder_api_vtbl);
      SETUP_VTBL(encoder_vtbl, x86_generic_okdb_wal_encoder_api_vtbl);
      break;
    case nehalem:
      SETUP_VTBL(decoder_vtbl, x86_nehalem_okdb_wal_decoder_api_vtbl);
      SETUP_VTBL(encoder_vtbl, x86_nehalem_okdb_wal_encoder_api_vtbl);
      break;
    case haswell:
      SETUP_VTBL(decoder_vtbl, x86_haswell_okdb_wal_decoder_api_vtbl);
      SETUP_VTBL(encoder_vtbl, x86_haswell_okdb_wal_encoder_api_vtbl);
      break;
#elif ARCH_DETECTOR_BUILD_FOR_ARM64
    default:
    case generic:
      SETUP_VTBL(decoder_vtbl, aarch64_generic_okdb_wal_decoder_api_vtbl);
      SETUP_VTBL(encoder_vtbl, aarch64_generic_okdb_wal_encoder_api_vtbl);
      break;
    case crc32:
      SETUP_VTBL(decoder_vtbl, aarch64_crc_okdb_wal_decoder_api_vtbl);
      SETUP_VTBL(encoder_vtbl, aarch64_crc_okdb_wal_encoder_api_vtbl);
      break;
#endif
  }
}

}  // namespace

extern "C" {

//
// decoder and types routed C API
//

// Decoder
// okdb_wal_c_decoder_ctor - constructor, C wrapper C++, init C++ class Decoder.
c_decoder okdb_wal_c_decoder_ctor() {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_decoder_ctor">(); }, decoder_vtbl);
}

// okdb_wal_c_decoder_decode - C wrapper C++, calls C++ class Decoder methods.
uint32_t okdb_wal_c_decoder_decode(c_decoder c_dec, c_slice_ptr c_seg, c_decoded_segment* c_protobuf) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_decoder_decode">(c_dec, c_seg, c_protobuf); }, decoder_vtbl);
}

// okdb_wal_c_decoder_decode_dry - C wrapper C++, calls C++ class Decoder methods.
uint32_t okdb_wal_c_decoder_decode_dry(c_decoder c_dec, c_slice_ptr c_seg) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_decoder_decode_dry">(c_dec, c_seg); }, decoder_vtbl);
}

// okdb_wal_c_decoder_snapshot - C wrapper C++, calls C++ class Decoder methods.
void okdb_wal_c_decoder_snapshot(c_decoder c_dec, c_slice_ptr c_snap) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_decoder_snapshot">(c_dec, c_snap); }, decoder_vtbl);
}

// okdb_wal_c_decoder_dtor - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_decoder_dtor(c_decoder c_dec) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_decoder_dtor">(c_dec); }, decoder_vtbl);
}  // encoder and types routed C API

// okdb_wal_c_redundant_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_redundant_destroy(c_redundant* c_rt) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_redundant_destroy">(c_rt); }, encoder_vtbl);
}

//
// Hashdex
// okdb_wal_c_hashdex_ctor - constructor, C wrapper C++, init C++ class Hashdex.
c_hashdex okdb_wal_c_hashdex_ctor() {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_hashdex_ctor">(); }, encoder_vtbl);
}

// okdb_wal_c_hashdex_presharding - C wrapper C++, calls C++ class Hashdex methods.
void okdb_wal_c_hashdex_presharding(c_hashdex c_hx, c_slice_ptr proto_data, c_slice_ptr cluster, c_slice_ptr replica) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_hashdex_presharding">(c_hx, proto_data, cluster, replica); }, encoder_vtbl);
}

// okdb_wal_c_hashdex_dtor - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_hashdex_dtor(c_hashdex c_hx) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_hashdex_dtor">(c_hx); }, encoder_vtbl);
}

//
// Encoder
// okdb_wal_c_encoder_ctor - constructor, C wrapper C++, init C++ class Encoder.
c_encoder okdb_wal_c_encoder_ctor(uint16_t shard_id, uint16_t number_of_shards) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_encoder_ctor">(shard_id, number_of_shards); }, encoder_vtbl);
}

// okdb_wal_c_encoder_encode - C wrapper C++, calls C++ class Encoder methods.
void okdb_wal_c_encoder_encode(c_encoder c_enc, c_hashdex c_hx, c_segment* c_seg, c_redundant* c_rt) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_encoder_encode">(c_enc, c_hx, c_seg, c_rt); }, encoder_vtbl);
}
void okdb_wal_c_encoder_add(c_encoder c_enc, c_hashdex c_hx, c_segment* c_seg) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_encoder_add">(c_enc, c_hx, c_seg); }, encoder_vtbl);
}
void okdb_wal_c_encoder_finalize(c_encoder c_enc, c_segment* c_seg, c_redundant* c_rt) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_encoder_finalize">(c_enc, c_seg, c_rt); }, encoder_vtbl);
}

// okdb_wal_c_encoder_snapshot - C wrapper C++, calls C++ class Encoder methods.
void okdb_wal_c_encoder_snapshot(c_encoder c_enc, c_slice_ptr c_rts, c_snapshot* c_snap) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_encoder_snapshot">(c_enc, c_rts, c_snap); }, encoder_vtbl);
}

// okdb_wal_c_encoder_dtor - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_encoder_dtor(c_encoder c_enc) {
  return std::visit([&](auto& vtbl) { return vtbl.template call<"okdb_wal_c_encoder_dtor">(c_enc); }, encoder_vtbl);
}

//
// Entry point
//

/// \brief Entry point for WAL C API. Call this function before
///        using any another.
/// \details
//           Initializes the arch-optimized version of C API, detecting the
//           supported architecture features at run-time.
/// \returns arch flavour ID.
int okdb_wal_initialize() {
  auto cur_arch_flavour = determine_arch_flavour();

  setup_arch_functions(cur_arch_flavour);
  return cur_arch_flavour;
}

}  // extern "C"
