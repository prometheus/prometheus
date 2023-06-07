#pragma once

#include "wal_c_api/wal_c_decoder.h"
#include "wal_c_api/wal_c_encoder.h"

#ifdef __cplusplus
extern "C" {
#endif

// Entry point

/// \brief Initializes all C API bindings (encoder, decoder, etc.).
/// \returns arch flavour ID.
int okdb_wal_initialize();
#ifdef __cplusplus
}
#endif
