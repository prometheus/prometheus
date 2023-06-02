/**
 * \file wal_c_decoder.h
 *  Wrapper C API for BasicReader<> class.
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
// okdb_wal_c_decoder_decode - C wrapper C++, calls C++ class Decoder methods.
uint32_t okdb_wal_c_decoder_decode(c_decoder c_dec, c_slice c_seg, c_slice_with_string_buffer* c_protobuf);
// okdb_wal_c_decoder_decode_dry - C wrapper C++, calls C++ class Decoder methods.
uint32_t okdb_wal_c_decoder_decode_dry(c_decoder c_dec, c_slice c_seg);
// okdb_wal_c_decoder_snapshot - C wrapper C++, calls C++ class Decoder methods.
void okdb_wal_c_decoder_snapshot(c_decoder c_dec, c_slice c_snap);
// okdb_wal_c_decoder_dtor - calls the destructor, C wrapper C++ for clear memory.
void okdb_wal_c_decoder_dtor(c_decoder c_dec);

#ifdef __cplusplus
}  // extern "C"
#endif
