#pragma once

#include "wal_c_api/wal_c_decoder.h"
#include "wal_c_api/wal_c_types.h"

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// decoder and types routed C API

// Decoder
// okdb_wal_c_decoder_ctor - constructor, C wrapper C++, init C++ class Decoder.
c_decoder okdb_wal_c_decoder_ctor();

void okdb_wal_uni_c_decoder_ctor(c_decoder* c_dec, c_api_error_info** err);

// okdb_wal_c_decoder_decode - C wrapper C++, calls C++ class Decoder methods.
uint32_t okdb_wal_c_decoder_decode(c_decoder c_dec, c_slice_ptr c_seg, c_decoded_segment* c_protobuf);

typedef struct {
  c_slice_ptr segment;
  c_decoded_segment* protobuf;
} c_decoder_decode_params;

typedef struct {
  uint32_t result;
} c_decoder_decode_result;

void okdb_wal_uni_c_decoder_decode(c_decoder c_dec, c_decoder_decode_params* params, c_decoder_decode_result* result, c_api_error_info** err);

// okdb_wal_c_decoder_decode_dry - C wrapper C++, calls C++ class Decoder methods.
uint32_t okdb_wal_c_decoder_decode_dry(c_decoder c_dec, c_slice_ptr c_seg);

typedef struct {
  c_slice_ptr segment;
} c_decoder_decode_dry_params;

void okdb_wal_uni_c_decoder_decode_dry(c_decoder c_dec, c_decoder_decode_dry_params* params, c_decoder_decode_result* result, c_api_error_info** err);

// okdb_wal_c_decoder_snapshot - C wrapper C++, calls C++ class Decoder methods.
void okdb_wal_c_decoder_snapshot(c_decoder c_dec, c_slice_ptr c_snap);

typedef struct {
  c_slice_ptr snapshot;
} c_decoder_snapshot_params;

void okdb_wal_uni_c_decoder_snapshot(c_decoder c_dec, c_decoder_snapshot_params* params, c_api_error_info** err);

// okdb_wal_c_decoder_dtor - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_decoder_dtor(c_decoder c_dec);

// Entry point

/// \returns arch flavour ID.
int okdb_wal_initialize();

// convenience macro for api set initialization.
#define okdb_wal_initialize_decoder okdb_wal_initialize
#ifdef __cplusplus
}  // extern "C"
#endif
