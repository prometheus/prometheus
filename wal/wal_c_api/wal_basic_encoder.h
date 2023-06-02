/**
 * \file basic_encoder.h
 *  Contains C API for \ref BasicEncoder<> class.
 *  \note You should set #define OKDB_WAL_FUNCTION_NAME_PREFIX before
 *  #include'ing this header for custom arch prefix.
 *  \note There are no include guard for allowing multiple #include-s
 *  with distinct OKDB_WAL_NAME_PREFIX
 */

#ifdef OKDB_WAL_API_SET_NAME
#undef OKDB_WAL_API_SET_NAME
#endif

#define OKDB_WAL_API_SET_NAME okdb_wal_writer
#include "wal_prefixed_name.h"

#ifndef OKDB_WAL_NAME_PREFIX
#define OKDB_WAL_NAME_PREFIX
#endif
#ifdef __cplusplus
extern "C" {
#endif

#ifndef OKDB_WAL_BASIC_WRITER_DEFINED
typedef void* basic_encoder_ptr;
#define OKDB_WAL_BASIC_WRITER_DEFINED
#endif

basic_encoder_ptr OKDB_WAL_PREFIXED_NAME(okdb_wal_basic_encoder_create)();

void OKDB_WAL_PREFIXED_NAME(okdb_wal_basic_encoder_destroy)(basic_encoder_ptr encoder);

#ifdef __cplusplus
}  // extern "C"

// export logic is avail only in C++
OKDB_WAL_EXPORT_API_BEGIN
OKDB_WAL_EXPORT_API(okdb_wal_basic_encoder_create)
OKDB_WAL_EXPORT_API(okdb_wal_basic_encoder_destroy)
OKDB_WAL_EXPORT_API_END

#endif
