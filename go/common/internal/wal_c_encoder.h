/**
 * \file wal_c_encoder.h
 *  Wrapper C API for BasicEncoder<> class.
 */
#include <stdint.h>

#include "wal_c_types.h"

#ifdef __cplusplus
extern "C" {
#endif

// c_redundant_ptr - redundant pointer.
typedef void* c_redundant_ptr;
// c_hashdex_ptr - hashdex pointer.
typedef void* c_hashdex_ptr;
// c_encoder_ptr - encoder pointer.
typedef void* c_encoder_ptr;

#ifndef OKDB_WAL_ENCODER_DEFINED

// c_redundant - C wrapper C++ Redundant for exchange memory between C++ and Go.
typedef struct {
  c_redundant_ptr data;
} c_redundant;

// c_hashdex - C wrapper C++, for init memory.
typedef c_hashdex_ptr c_hashdex;

// c_encoder - C wrapper C++, for init memory.
typedef c_encoder_ptr c_encoder;

#define OKDB_WAL_ENCODER_DEFINED
#endif

// Redundant
// okdb_wal_c_redundant_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_redundant_destroy(c_redundant* c_rt);

// Hashdex
// okdb_wal_c_hashdex_ctor - constructor, C wrapper C++, init C++ class Hashdex.
c_hashdex okdb_wal_c_hashdex_ctor();

void okdb_wal_uni_c_hashdex_ctor(c_hashdex* out_hashdex_ptr, c_api_error_info** err);

// okdb_wal_c_hashdex_presharding - C wrapper C++, calls C++ class Hashdex methods.
void okdb_wal_c_hashdex_presharding(c_hashdex c_hx, c_slice_from_go_ptr proto_data, c_slice_from_go_ptr cluster, c_slice_from_go_ptr replica);

void okdb_wal_uni_c_hashdex_presharding(c_hashdex c_hx, go_ptr params, go_ptr err);

// okdb_wal_c_hashdex_dtor - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_hashdex_dtor(c_hashdex c_hx);

// Encoder
// okdb_wal_c_encoder_ctor - constructor, C wrapper C++, init C++ class Encoder.
c_encoder okdb_wal_c_encoder_ctor(uint16_t shard_id, uint16_t number_of_shards);

typedef struct {
  uint16_t shard_id, number_of_shards;
} c_encoder_ctor_params;

void okdb_wal_uni_c_encoder_ctor(go_ptr in_ctor_args, go_ptr out_encoder_ptr, go_ptr err);

// okdb_wal_c_encoder_encode - C wrapper C++, calls C++ class Encoder methods.
void okdb_wal_c_encoder_encode(c_encoder c_enc, c_hashdex c_hx, c_segment* c_seg, c_redundant* c_rt);

typedef struct {
  c_hashdex hashdex;
  c_segment* segment;
  c_redundant* redundant;
} c_encoder_encode_params;

void okdb_wal_uni_c_encoder_encode(go_ptr c_enc, go_ptr encode_params, go_ptr err);

// okdb_wal_c_encoder_snapshot - C wrapper C++, calls C++ class Encoder methods.
void okdb_wal_c_encoder_snapshot(c_encoder c_enc, c_slice_from_go_ptr c_rts, c_snapshot* c_snap);

typedef struct {
  c_slice_ptr c_rts;
  c_snapshot c_snapshot;
} c_encoder_snapshot_params;

void okdb_wal_uni_c_encoder_snapshot(go_ptr c_enc, go_ptr snapshot_params, go_ptr err);
// okdb_wal_c_encoder_dtor - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_encoder_dtor(c_encoder c_enc);

#ifdef __cplusplus
}  // extern "C"
#endif
