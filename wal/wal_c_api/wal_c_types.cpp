#include "wal_c_types.h"

#include <sstream>

extern "C" {
/**
 * Factory for destroy buffer in slice
 */

// okdb_wal_c_slice_with_stream_buffer_destroy - calls the destructor, C wrapper C++ for clear memory.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_slice_with_stream_buffer_destroy)(c_slice_with_stream_buffer* c_segment) {
  delete static_cast<std::stringstream*>(c_segment->buf);
}

// okdb_wal_c_slice_with_string_buffer_destroy - calls the destructor, C wrapper C++ for clear memory.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_slice_with_string_buffer_destroy)(c_slice_with_string_buffer* c_segment) {
  delete static_cast<std::string*>(c_segment->buf);
}

}  // extern "C"
