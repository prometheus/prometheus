#pragma once

#include "wal_c_api/wal_c_encoder.h"
#include "wal_c_api/wal_c_types.h"

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// encoder and types routed C API

// Redundant
// okdb_wal_c_redundant_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_redundant_destroy(c_redundant* c_rt);

// Hashdex
// okdb_wal_c_hashdex_ctor - constructor, C wrapper C++, init C++ class Hashdex.
c_hashdex okdb_wal_c_hashdex_ctor();

void okdb_wal_uni_c_hashdex_ctor(c_hashdex* out_hashdex_ptr, c_api_error_info** err);

// okdb_wal_c_hashdex_presharding - C wrapper C++, calls C++ class Hashdex methods.
void okdb_wal_c_hashdex_presharding(c_hashdex c_hx, c_slice_ptr proto_data, c_slice_ptr cluster, c_slice_ptr replica);

typedef struct {
  c_slice_ptr proto_data;
  c_slice_ptr cluster;
  c_slice_ptr replica;
} c_hashdex_presharding_params;

void okdb_wal_uni_c_hashdex_presharding(c_hashdex c_hx, c_hashdex_presharding_params* params, c_api_error_info** err);

// okdb_wal_c_hashdex_dtor - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_hashdex_dtor(c_hashdex c_hx);

// Encoder
// okdb_wal_c_encoder_ctor - constructor, C wrapper C++, init C++ class Encoder.
c_encoder okdb_wal_c_encoder_ctor(uint16_t shard_id, uint16_t number_of_shards);

typedef struct {
  uint16_t shard_id, number_of_shards;
} c_encoder_ctor_params;

void okdb_wal_uni_c_encoder_ctor(c_encoder_ctor_params* in_ctor_args, c_encoder* out_encoder_ptr, c_api_error_info** err);

// okdb_wal_c_encoder_encode - C wrapper C++, calls C++ class Encoder methods.
void okdb_wal_c_encoder_encode(c_encoder c_enc, c_hashdex c_hx, c_segment* c_seg, c_redundant* c_rt);

typedef struct {
  c_hashdex hashdex;
  c_segment* segment;
  c_redundant* redundant;
} c_encoder_encode_params;

void okdb_wal_uni_c_encoder_encode(c_encoder c_enc, c_encoder_encode_params* encode_params, c_api_error_info** err);

// okdb_wal_c_encoder_add - C wrapper C++, calls C++ class Encoder methods.
void okdb_wal_c_encoder_add(c_encoder c_enc, c_hashdex c_hx, c_segment* c_seg);

typedef struct {
  c_hashdex hashdex;
  c_segment* segment;
} c_encoder_add_params;

void okdb_wal_uni_c_encoder_add(c_encoder c_enc, c_encoder_add_params* add_params, c_api_error_info** err);

// okdb_wal_c_encoder_finalize - C wrapper C++, calls C++ class Encoder methods.
void okdb_wal_c_encoder_finalize(c_encoder c_enc, c_segment* c_seg, c_redundant* c_rt);

typedef struct {
  c_segment* segment;
  c_redundant* redundant;
} c_encoder_finalize_params;

void okdb_wal_uni_c_encoder_finalize(c_encoder c_enc, c_encoder_finalize_params* finalize_params, c_api_error_info** err);

// okdb_wal_c_encoder_snapshot - C wrapper C++, calls C++ class Encoder methods.
void okdb_wal_c_encoder_snapshot(c_encoder c_enc, c_slice_ptr c_rts, c_snapshot* c_snap);

typedef struct {
  c_slice_ptr c_rts;
  c_snapshot* snapshot;
} c_encoder_snapshot_params;

void okdb_wal_uni_c_encoder_snapshot(c_encoder c_enc, c_encoder_snapshot_params* snapshot_params, c_api_error_info** err);

// okdb_wal_c_encoder_dtor - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_encoder_dtor(c_encoder c_enc);

// Entry point

/// \returns arch flavour ID.
int okdb_wal_initialize();

// convenience macro for api set initialization.
#define okdb_wal_initialize_encoder okdb_wal_initialize
#ifdef __cplusplus
}  // extern "C"
#endif
