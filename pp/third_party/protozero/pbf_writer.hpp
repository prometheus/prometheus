#ifndef PROTOZERO_PBF_WRITER_HPP
#define PROTOZERO_PBF_WRITER_HPP

/*****************************************************************************

protozero - Minimalistic protocol buffer decoder and encoder in C++.

This file is from https://github.com/mapbox/protozero where you can find more
documentation.

*****************************************************************************/

/**
 * @file pbf_writer.hpp
 *
 * @brief Contains the pbf_writer class.
 */

#include "basic_pbf_writer.hpp"
#include "buffer_string.hpp"

#include <cstdint>
#include <string>

namespace protozero {

/**
 * Specialization of basic_pbf_writer using std::string as buffer type.
 */
using pbf_writer = basic_pbf_writer<std::string>;

/// Class for generating packed repeated bool fields.
using packed_field_bool     = detail::packed_field_varint<std::string, bool>;

/// Class for generating packed repeated enum fields.
using packed_field_enum     = detail::packed_field_varint<std::string, int32_t>;

/// Class for generating packed repeated int32 fields.
using packed_field_int32    = detail::packed_field_varint<std::string, int32_t>;

/// Class for generating packed repeated sint32 fields.
using packed_field_sint32   = detail::packed_field_svarint<std::string, int32_t>;

/// Class for generating packed repeated uint32 fields.
using packed_field_uint32   = detail::packed_field_varint<std::string, uint32_t>;

/// Class for generating packed repeated int64 fields.
using packed_field_int64    = detail::packed_field_varint<std::string, int64_t>;

/// Class for generating packed repeated sint64 fields.
using packed_field_sint64   = detail::packed_field_svarint<std::string, int64_t>;

/// Class for generating packed repeated uint64 fields.
using packed_field_uint64   = detail::packed_field_varint<std::string, uint64_t>;

/// Class for generating packed repeated fixed32 fields.
using packed_field_fixed32  = detail::packed_field_fixed<std::string, uint32_t>;

/// Class for generating packed repeated sfixed32 fields.
using packed_field_sfixed32 = detail::packed_field_fixed<std::string, int32_t>;

/// Class for generating packed repeated fixed64 fields.
using packed_field_fixed64  = detail::packed_field_fixed<std::string, uint64_t>;

/// Class for generating packed repeated sfixed64 fields.
using packed_field_sfixed64 = detail::packed_field_fixed<std::string, int64_t>;

/// Class for generating packed repeated float fields.
using packed_field_float    = detail::packed_field_fixed<std::string, float>;

/// Class for generating packed repeated double fields.
using packed_field_double   = detail::packed_field_fixed<std::string, double>;

} // end namespace protozero

#endif // PROTOZERO_PBF_WRITER_HPP
