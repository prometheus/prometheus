#ifndef PROTOZERO_TYPES_HPP
#define PROTOZERO_TYPES_HPP

/*****************************************************************************

protozero - Minimalistic protocol buffer decoder and encoder in C++.

This file is from https://github.com/mapbox/protozero where you can find more
documentation.

*****************************************************************************/

/**
 * @file types.hpp
 *
 * @brief Contains the declaration of low-level types used in the pbf format.
 */

#include "config.hpp"

#include <algorithm>
#include <cstddef>
#include <cstdint>
#include <cstring>
#include <string>
#include <utility>

namespace protozero {

/**
 * The type used for field tags (field numbers).
 */
using pbf_tag_type = uint32_t;

/**
 * The type used to encode type information.
 * See the table on
 *    https://developers.google.com/protocol-buffers/docs/encoding
 */
enum class pbf_wire_type : uint32_t {
    varint           = 0, // int32/64, uint32/64, sint32/64, bool, enum
    fixed64          = 1, // fixed64, sfixed64, double
    length_delimited = 2, // string, bytes, nested messages, packed repeated fields
    fixed32          = 5, // fixed32, sfixed32, float
    unknown          = 99 // used for default setting in this library
};

/**
 * Get the tag and wire type of the current field in one integer suitable
 * for comparison with a switch statement.
 *
 * See pbf_reader.tag_and_type() for an example how to use this.
 */
template <typename T>
constexpr inline uint32_t tag_and_type(T tag, pbf_wire_type wire_type) noexcept {
    return (static_cast<uint32_t>(static_cast<pbf_tag_type>(tag)) << 3U) | static_cast<uint32_t>(wire_type);
}

/**
 * The type used for length values, such as the length of a field.
 */
using pbf_length_type = uint32_t;

} // end namespace protozero

#endif // PROTOZERO_TYPES_HPP
