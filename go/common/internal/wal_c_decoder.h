/**
 * \file wal_c_decoder.h
 *  Wrapper C API for BasicDecoder<> class.
 */
#include <stdint.h>

#include "wal_c_types.h"

#ifdef __cplusplus
extern "C" {
#endif

// c_decoder_ptr - encoder pointer.
typedef void* c_decoder_ptr;

#ifndef OKDB_WAL_DECODER_DEFINED

// c_decoder - C wrapper C++, for init memory.
typedef c_decoder_ptr c_decoder;

#define OKDB_WAL_DECODER_DEFINED
#endif

// Decoder
// okdb_wal_c_decoder_ctor - constructor, C wrapper C++, init C++ class Decoder.
c_decoder okdb_wal_c_decoder_ctor();

void okdb_wal_uni_c_decoder_ctor(c_decoder* c_dec, c_api_error_info** err);

// okdb_wal_c_decoder_decode - C wrapper C++, calls C++ class Decoder methods.
uint32_t okdb_wal_c_decoder_decode(c_decoder c_dec, c_slice_from_go_ptr c_seg, c_decoded_segment* c_protobuf);

void okdb_wal_uni_c_decoder_decode(c_decoder c_dec, go_ptr params, go_ptr result, go_ptr err);

// okdb_wal_c_decoder_decode_dry - C wrapper C++, calls C++ class Decoder methods.
uint32_t okdb_wal_c_decoder_decode_dry(c_decoder c_dec, c_slice_from_go_ptr c_seg);

void okdb_wal_uni_c_decoder_decode_dry(c_decoder c_dec, go_ptr params, go_ptr result, go_ptr err);

// okdb_wal_c_decoder_snapshot - C wrapper C++, calls C++ class Decoder methods.
void okdb_wal_c_decoder_snapshot(c_decoder c_dec, c_slice_from_go_ptr c_snap);

void okdb_wal_uni_c_decoder_snapshot(c_decoder c_dec, go_ptr params, go_ptr err);

// okdb_wal_c_decoder_dtor - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_decoder_dtor(c_decoder c_dec);

#ifdef __cplusplus
}  // extern "C"
#endif
