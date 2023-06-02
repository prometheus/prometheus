#pragma once

#include "wal_c_api/wal_c_types.h"

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// c types routed C API

// dtor
// okdb_wal_c_slice_with_stream_buffer_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_slice_with_stream_buffer_destroy(c_slice_with_stream_buffer* c_segment);

// okdb_wal_c_slice_with_string_buffer_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_slice_with_string_buffer_destroy(c_slice_with_string_buffer* c_segment);

// Entry point

/// \returns arch flavour ID.
int okdb_wal_initialize_types();
#ifdef __cplusplus
}
#endif
