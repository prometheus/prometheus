#pragma once

#include "wal_c_api/wal_basic_decoder.h"
#include "wal_c_api/wal_basic_encoder.h"

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// basic decoder (reader) routed C API

// ctor
basic_decoder_ptr okdb_wal_basic_decoder_create();
// getters
//
uint32_t okdb_wal_basic_decoder_get_series_count(basic_decoder_ptr reader);

uint32_t okdb_wal_basic_decoder_get_samples_count(basic_decoder_ptr reader);

// dtor
void okdb_wal_basic_decoder_destroy(basic_decoder_ptr reader);

// basic encoder (writer) routed C API

basic_encoder_ptr okdb_wal_basic_encoder_create();

void okdb_wal_basic_encoder_destroy(basic_encoder_ptr writer);

// Entry point

/// \returns arch flavour ID.
int okdb_wal_initialize();
#ifdef __cplusplus
}
#endif
