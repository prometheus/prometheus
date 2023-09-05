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

// Debug API (also declared in bare_bones/exception.h)

/// \brief Use it for enabling coredumps on any \ref BareBones::Exception.
/// \param enable Enables if != 0, disables otherwise.
extern "C" void prompp_enable_coredumps_on_exception(int enable);
#ifdef __cplusplus
}
#endif
