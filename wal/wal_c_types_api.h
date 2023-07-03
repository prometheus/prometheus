/**
 * \file wal/wal_c_types_api.h
 * Contains bridged Go <-> C types API declarations.
 *
 */
#pragma once

#include "wal_c_api/wal_c_types.h"

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// c types routed C API

// dtor
// okdb_wal_c_snapshot_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_snapshot_destroy(c_snapshot* snapshot);

// okdb_wal_c_segment_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_segment_destroy(c_segment* segment);

// okdb_wal_c_decoded_segment_destroy - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_decoded_segment_destroy(c_decoded_segment* decoded_segment);

// Entry point

/// \returns arch flavour ID.
int okdb_wal_initialize_types();
#ifdef __cplusplus
}
#endif
