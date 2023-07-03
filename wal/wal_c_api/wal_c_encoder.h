/**
 * \file wal_c_encoder.h
 *  Wrapper C API for BasicWriter<> class.
 *  \note You should set #define OKDB_WAL_FUNCTION_NAME_PREFIX before
 *  #include'ing this header for custom arch prefix.
 *  \note There are no include guard for allowing multiple #include-s
 *  with distinct OKDB_WAL_NAME_PREFIX
 */
#include <stdint.h>

#include "wal_c_types.h"

#ifdef __cplusplus

#ifdef OKDB_WAL_API_SET_NAME
#undef OKDB_WAL_API_SET_NAME
#endif
#define OKDB_WAL_API_SET_NAME okdb_wal_encoder

#include "wal_prefixed_name.h"
extern "C" {
#else
#define OKDB_WAL_PREFIXED_NAME(str) str
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
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_redundant_destroy)(c_redundant* c_rt);

// Hashdex
// okdb_wal_c_hashdex_ctor - constructor, C wrapper C++, init C++ class Hashdex.
c_hashdex OKDB_WAL_PREFIXED_NAME(okdb_wal_c_hashdex_ctor)();
// okdb_wal_c_hashdex_presharding - C wrapper C++, calls C++ class Hashdex methods.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_hashdex_presharding)(c_hashdex c_hx, c_slice proto_data);
// okdb_wal_c_hashdex_dtor - calls the destructor, C wrapper C++ for clear memory.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_hashdex_dtor)(c_hashdex c_hx);

// Encoder
// okdb_wal_c_encoder_ctor - constructor, C wrapper C++, init C++ class Encoder.
c_encoder OKDB_WAL_PREFIXED_NAME(okdb_wal_c_encoder_ctor)(uint16_t shard_id, uint16_t number_of_shards);
// okdb_wal_c_encoder_encode - C wrapper C++, calls C++ class Encoder methods.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_encoder_encode)(c_encoder c_enc, c_hashdex c_hx, c_segment* c_seg, c_redundant* c_rt);
// okdb_wal_c_encoder_snapshot - C wrapper C++, calls C++ class Encoder methods.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_encoder_snapshot)(c_encoder c_enc, c_slice c_rts, c_snapshot* c_snap);
// okdb_wal_c_encoder_dtor - calls the destructor, C wrapper C++ for clear memory.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_encoder_dtor)(c_encoder c_enc);

#ifdef __cplusplus
}  // extern "C"

OKDB_WAL_EXPORT_API_BEGIN
OKDB_WAL_EXPORT_API(okdb_wal_c_redundant_destroy)
OKDB_WAL_EXPORT_API(okdb_wal_c_hashdex_ctor)
OKDB_WAL_EXPORT_API(okdb_wal_c_hashdex_presharding)
OKDB_WAL_EXPORT_API(okdb_wal_c_hashdex_dtor)
OKDB_WAL_EXPORT_API(okdb_wal_c_encoder_ctor)
OKDB_WAL_EXPORT_API(okdb_wal_c_encoder_encode)
OKDB_WAL_EXPORT_API(okdb_wal_c_encoder_snapshot)
OKDB_WAL_EXPORT_API(okdb_wal_c_encoder_dtor)
OKDB_WAL_EXPORT_API_END

#endif
