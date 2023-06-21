/**
 * \file wal_c_types.h
 *  Contains C API for types CGO class.
 */
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
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
void okdb_wal_c_slice_with_stream_buffer_destroy(c_slice_with_stream_buffer* c_segment);

// okdb_wal_c_slice_with_string_buffer_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_slice_with_string_buffer_destroy(c_slice_with_string_buffer* c_segment);

// okdb_wal_initialize - entry point for C bindings.
int okdb_wal_initialize();

#ifdef __cplusplus
}  // extern "C"
#endif
