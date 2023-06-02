/**
 * \file wal_basic_decoder.h
 *  Contains C API for \ref BasicDecoder<> class.
 *  \note You should set #define OKDB_WAL_FUNCTION_NAME_PREFIX before
 *  #include'ing this header for custom arch prefix.
 *  \note There are no include guard for allowing multiple #include-s
 *  with distinct \ref OKDB_WAL_NAME_PREFIX
 */
#include <stdint.h>

#ifdef OKDB_WAL_API_SET_NAME
#undef OKDB_WAL_API_SET_NAME
#endif

#define OKDB_WAL_API_SET_NAME okdb_wal_reader
#include "wal_prefixed_name.h"

#ifdef __cplusplus
extern "C" {
#endif

#ifndef OKDB_WAL_BASIC_READER_DEFINED
typedef void* basic_decoder_ptr;
#define OKDB_WAL_BASIC_READER_DEFINED
#endif

// ctor
basic_decoder_ptr OKDB_WAL_PREFIXED_NAME(okdb_wal_basic_decoder_create)();

// getters
//
uint32_t OKDB_WAL_PREFIXED_NAME(okdb_wal_basic_decoder_get_series_count)(basic_decoder_ptr decoder);

uint32_t OKDB_WAL_PREFIXED_NAME(okdb_wal_basic_decoder_get_samples_count)(basic_decoder_ptr decoder);

// dtor
void OKDB_WAL_PREFIXED_NAME(okdb_wal_basic_decoder_destroy)(basic_decoder_ptr decoder);

#ifdef __cplusplus
}  // extern "C"

OKDB_WAL_EXPORT_API_BEGIN
OKDB_WAL_EXPORT_API(okdb_wal_basic_decoder_create)
OKDB_WAL_EXPORT_API(okdb_wal_basic_decoder_get_series_count)
OKDB_WAL_EXPORT_API(okdb_wal_basic_decoder_get_samples_count)
OKDB_WAL_EXPORT_API(okdb_wal_basic_decoder_destroy)
OKDB_WAL_EXPORT_API_END

#endif
