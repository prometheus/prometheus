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

typedef struct {
  c_slice data;
  stringstream_ptr buf;
} c_snapshot;

typedef struct {
  c_slice data;
  stringstream_ptr buf;
  uint32_t samples;
  uint32_t series;
  int64_t earliest_timestamp;
  int64_t latest_timestamp;
} c_segment;

typedef struct {
  c_slice data;
  string_ptr buf;
  int64_t created_at;
  int64_t encoded_at;
} c_decoded_segment;
#define OKDB_WAL_TYPES_DEFINED
#endif

// dtor
// okdb_wal_c_snapshot_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_snapshot_destroy(c_snapshot* snapshot);

// okdb_wal_c_segment_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_segment_destroy(c_segment* segment);

// okdb_wal_c_decoded_segment_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_decoded_segment_destroy(c_decoded_segment* decoded_segment);

// okdb_wal_initialize - entry point for C bindings.
int okdb_wal_initialize();

#ifdef __cplusplus
}  // extern "C"
#endif
