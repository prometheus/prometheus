/**
 * \file wal_c_types.h
 *  Contains C API for types CGO class.
 *  \note You should set #define OKDB_WAL_FUNCTION_NAME_PREFIX before
 *  #include'ing this header for custom arch prefix.
 *  \note There are no include guard for allowing multiple #include-s
 *  with distinct OKDB_WAL_NAME_PREFIX
 */
#pragma once
#include <stddef.h>  // size_t
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef void* stringstream_ptr;
typedef void* string_ptr;

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

// dtor
// okdb_wal_c_slice_with_stream_buffer_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_slice_with_stream_buffer_destroy(c_slice_with_stream_buffer* c_segment);

// okdb_wal_c_slice_with_string_buffer_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_slice_with_string_buffer_destroy(c_slice_with_string_buffer* c_segment);

#ifdef __cplusplus
}  // extern "C"
#endif
