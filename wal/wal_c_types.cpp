#include "wal_c_api/wal_c_types.h"

#include <sstream>
#include <string>

extern "C" {
/**
 * Factory for destroy buffer in slice
 */

// okdb_wal_c_snapshot_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_snapshot_destroy(c_snapshot* snapshot) {
  delete static_cast<std::stringstream*>(snapshot->buf);
}

// okdb_wal_c_segment_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_segment_destroy(c_segment* segment) {
  delete static_cast<std::stringstream*>(segment->buf);
}

// okdb_wal_c_decoded_segment_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_decoded_segment_destroy(c_decoded_segment* decoded_segment) {
  delete static_cast<std::string*>(decoded_segment->buf);
}

// std::string C bindings
const char* std_string_c_str(const string_ptr str) {
  if (str) {
    return static_cast<const std::string*>(str)->c_str();
  }
  return "";
}

size_t std_string_length(const string_ptr str) {
  if (str) {
    return static_cast<const std::string*>(str)->size();
  }
  return 0;
}

}  // extern "C"
