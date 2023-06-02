#pragma once

#include "wal_c_api/wal_c_decoder.h"

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// decoder and types routed C API

// Decoder
// okdb_wal_c_decoder_ctor - constructor, C wrapper C++, init C++ class Decoder.
c_decoder okdb_wal_c_decoder_ctor();
// okdb_wal_c_decoder_decode - C wrapper C++, calls C++ class Decoder methods.
uint32_t okdb_wal_c_decoder_decode(c_decoder c_dec, c_slice c_seg, c_slice_with_string_buffer* c_protobuf);
// okdb_wal_c_decoder_decode_dry - C wrapper C++, calls C++ class Decoder methods.
uint32_t okdb_wal_c_decoder_decode_dry(c_decoder c_dec, c_slice c_seg);
// okdb_wal_c_decoder_snapshot - C wrapper C++, calls C++ class Decoder methods.
void okdb_wal_c_decoder_snapshot(c_decoder c_dec, c_slice c_snap);
// okdb_wal_c_decoder_dtor - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_decoder_dtor(c_decoder c_dec);

// Entry point

/// \returns arch flavour ID.
int okdb_wal_initialize_decoder();
#ifdef __cplusplus
}
#endif
