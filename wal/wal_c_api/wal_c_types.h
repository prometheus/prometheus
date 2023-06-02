/**
 * \file wal_c_types.h
 *  Contains C API for types CGO class.
 *  \note You should set #define OKDB_WAL_FUNCTION_NAME_PREFIX before
 *  #include'ing this header for custom arch prefix.
 *  \note There are no include guard for allowing multiple #include-s
 *  with distinct OKDB_WAL_NAME_PREFIX
 */
#include <stdint.h>

#ifdef OKDB_WAL_API_SET_NAME
#define OKDB_WAL_API_SET_NAME_OLD_IMPL OKDB_WAL_API_SET_NAME
#define OKDB_WAL_API_SET_NAME_OLD OKDB_WAL_API_SET_NAME_OLD_IMPL
#undef OKDB_WAL_API_SET_NAME
#endif

#define OKDB_WAL_API_SET_NAME okdb_wal_types

#ifdef __cplusplus
#include "wal_prefixed_name.h"
extern "C" {
#else
#define OKDB_WAL_PREFIXED_NAME(str) str
#endif

typedef void* stringstream_ptr;
typedef void* string_ptr;

#ifndef OKDB_WAL_TYPES_DEFINED
// c_slice - C wrapper C++ Slice for exchange memory between C++ and Go.
typedef struct {
  const void* array;
  size_t len;
  size_t cap;
} c_slice;

// c_slice_with_stream_buffer - C wrapper C++ Segment and Snapshot for exchange memory
// between C++ and Go.
typedef struct {
  c_slice data;
  stringstream_ptr buf;
} c_slice_with_stream_buffer;

// c_slice_with_string_buffer - C wrapper C++ Segment and Snapshot for exchange memory
// between C++ and Go.
typedef struct {
  c_slice data;
  string_ptr buf;
} c_slice_with_string_buffer;
#define OKDB_WAL_TYPES_DEFINED
#endif

// dtor
// okdb_wal_c_slice_with_stream_buffer_destroy - calls the destructor, C wrapper C++ for clear memory.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_slice_with_stream_buffer_destroy)(c_slice_with_stream_buffer* c_segment);

// okdb_wal_c_slice_with_string_buffer_destroy - calls the destructor, C wrapper C++ for clear memory.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_slice_with_string_buffer_destroy)(c_slice_with_string_buffer* c_segment);

#ifdef __cplusplus
}  // extern "C"

OKDB_WAL_EXPORT_API_BEGIN
OKDB_WAL_EXPORT_API(okdb_wal_c_slice_with_stream_buffer_destroy)
OKDB_WAL_EXPORT_API(okdb_wal_c_slice_with_string_buffer_destroy)
OKDB_WAL_EXPORT_API_END

#endif

#ifdef OKDB_WAL_API_SET_NAME_OLD
#undef OKDB_WAL_API_SET_NAME
#define OKDB_WAL_API_SET_NAME OKDB_WAL_API_SET_NAME_OLD
#undef OKDB_WAL_API_SET_NAME_OLD
#undef OKDB_WAL_API_SET_NAME_OLD_IMPL
#endif
