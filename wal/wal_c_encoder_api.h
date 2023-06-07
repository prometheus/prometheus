#pragma once

#include "wal_c_api/wal_c_encoder.h"

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
// okdb_wal_c_hashdex_presharding - C wrapper C++, calls C++ class Hashdex methods.
void okdb_wal_c_hashdex_presharding(c_hashdex c_hx, c_slice proto_data);
// okdb_wal_c_hashdex_dtor - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_hashdex_dtor(c_hashdex c_hx);

// Encoder
// okdb_wal_c_encoder_ctor - constructor, C wrapper C++, init C++ class Encoder.
c_encoder okdb_wal_c_encoder_ctor(uint16_t shard_id, uint16_t number_of_shards);
// okdb_wal_c_encoder_encode - C wrapper C++, calls C++ class Encoder methods.
void okdb_wal_c_encoder_encode(c_encoder c_enc, c_hashdex c_hx, c_slice_with_stream_buffer* c_seg, c_redundant* c_rt);
// okdb_wal_c_encoder_snapshot - C wrapper C++, calls C++ class Encoder methods.
void okdb_wal_c_encoder_snapshot(c_encoder c_enc, c_slice c_rts, c_slice_with_stream_buffer* c_snap);
// okdb_wal_c_encoder_dtor - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_encoder_dtor(c_encoder c_enc);

// Entry point

/// \returns arch flavour ID.
int okdb_wal_initialize();

// convenience macro for api set initialization.
#define okdb_wal_initialize_encoder okdb_wal_initialize
#ifdef __cplusplus
}
#endif
