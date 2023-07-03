/**
 * \file wal_c_decoder.h
 *  Wrapper C API for BasicReader<> class.
 *  \note You should set #define OKDB_WAL_FUNCTION_NAME_PREFIX before
 *  #include'ing this header for custom arch prefix.
 *  \note There are no include guard for allowing multiple #include-s
 *  with distinct OKDB_WAL_NAME_PREFIX
 */
#include <stdint.h>

#include "wal_c_types.h"

#ifdef __cplusplus

#ifdef OKDB_WAL_API_SET_NAME
#undef OKDB_WAL_API_SET_NAME
#endif
#define OKDB_WAL_API_SET_NAME okdb_wal_decoder

#include "wal_prefixed_name.h"
extern "C" {
#else
#define OKDB_WAL_PREFIXED_NAME(str) str
#endif

// c_decoder_ptr - encoder pointer.
typedef void* c_decoder_ptr;
// c_decoder - C wrapper C++, for init memory.
typedef c_decoder_ptr c_decoder;

// Decoder
// okdb_wal_c_decoder_ctor - constructor, C wrapper C++, init C++ class Decoder.
c_decoder OKDB_WAL_PREFIXED_NAME(okdb_wal_c_decoder_ctor)();
// okdb_wal_c_decoder_decode - C wrapper C++, calls C++ class Decoder methods.
uint32_t OKDB_WAL_PREFIXED_NAME(okdb_wal_c_decoder_decode)(c_decoder c_dec, c_slice c_seg, c_decoded_segment* c_protobuf);
// okdb_wal_c_decoder_decode_dry - C wrapper C++, calls C++ class Decoder methods.
uint32_t OKDB_WAL_PREFIXED_NAME(okdb_wal_c_decoder_decode_dry)(c_decoder c_dec, c_slice c_seg);
// okdb_wal_c_decoder_snapshot - C wrapper C++, calls C++ class Decoder methods.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_decoder_snapshot)(c_decoder c_dec, c_slice c_snap);
// okdb_wal_c_decoder_dtor - calls the destructor, C wrapper C++ for clear memory.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_decoder_dtor)(c_decoder c_dec);

#ifdef __cplusplus
}  // extern "C"

OKDB_WAL_EXPORT_API_BEGIN
OKDB_WAL_EXPORT_API(okdb_wal_c_decoder_ctor)
OKDB_WAL_EXPORT_API(okdb_wal_c_decoder_decode)
OKDB_WAL_EXPORT_API(okdb_wal_c_decoder_decode_dry)
OKDB_WAL_EXPORT_API(okdb_wal_c_decoder_snapshot)
OKDB_WAL_EXPORT_API(okdb_wal_c_decoder_dtor)
OKDB_WAL_EXPORT_API_END

#endif
