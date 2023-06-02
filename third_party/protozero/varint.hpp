#ifndef PROTOZERO_VARINT_HPP
#define PROTOZERO_VARINT_HPP

/*****************************************************************************

protozero - Minimalistic protocol buffer decoder and encoder in C++.

This file is from https://github.com/mapbox/protozero where you can find more
documentation.

*****************************************************************************/

/**
 * @file varint.hpp
 *
 * @brief Contains low-level varint and zigzag encoding and decoding functions.
 */

#include "buffer_tmpl.hpp"
#include "exception.hpp"

#include <cstdint>

namespace protozero {

/**
 * The maximum length of a 64 bit varint.
 */
constexpr const int8_t max_varint_length = sizeof(uint64_t) * 8 / 7 + 1;

namespace detail {

    // from https://github.com/facebook/folly/blob/master/folly/Varint.h
    inline uint64_t decode_varint_impl(const char** data, const char* end) {
        const auto* begin = reinterpret_cast<const int8_t*>(*data);
        const auto* iend = reinterpret_cast<const int8_t*>(end);
        const int8_t* p = begin;
        uint64_t val = 0;

        if (iend - begin >= max_varint_length) {  // fast path
            do {
                int64_t b = *p++;
                          val  = ((uint64_t(b) & 0x7fU)       ); if (b >= 0) { break; }
                b = *p++; val |= ((uint64_t(b) & 0x7fU) <<  7U); if (b >= 0) { break; }
                b = *p++; val |= ((uint64_t(b) & 0x7fU) << 14U); if (b >= 0) { break; }
                b = *p++; val |= ((uint64_t(b) & 0x7fU) << 21U); if (b >= 0) { break; }
                b = *p++; val |= ((uint64_t(b) & 0x7fU) << 28U); if (b >= 0) { break; }
                b = *p++; val |= ((uint64_t(b) & 0x7fU) << 35U); if (b >= 0) { break; }
                b = *p++; val |= ((uint64_t(b) & 0x7fU) << 42U); if (b >= 0) { break; }
                b = *p++; val |= ((uint64_t(b) & 0x7fU) << 49U); if (b >= 0) { break; }
                b = *p++; val |= ((uint64_t(b) & 0x7fU) << 56U); if (b >= 0) { break; }
                b = *p++; val |= ((uint64_t(b) & 0x01U) << 63U); if (b >= 0) { break; }
                throw varint_too_long_exception{};
            } while (false);
        } else {
            unsigned int shift = 0;
            while (p != iend && *p < 0) {
                val |= (uint64_t(*p++) & 0x7fU) << shift;
                shift += 7;
            }
            if (p == iend) {
                throw end_of_buffer_exception{};
            }
            val |= uint64_t(*p++) << shift;
        }

        *data = reinterpret_cast<const char*>(p);
        return val;
    }

} // end namespace detail

/**
 * Decode a 64 bit varint.
 *
 * Strong exception guarantee: if there is an exception the data pointer will
 * not be changed.
 *
 * @param[in,out] data Pointer to pointer to the input data. After the function
 *        returns this will point to the next data to be read.
 * @param[in] end Pointer one past the end of the input data.
 * @returns The decoded integer
 * @throws varint_too_long_exception if the varint is longer then the maximum
 *         length that would fit in a 64 bit int. Usually this means your data
 *         is corrupted or you are trying to read something as a varint that
 *         isn't.
 * @throws end_of_buffer_exception if the *end* of the buffer was reached
 *         before the end of the varint.
 */
inline uint64_t decode_varint(const char** data, const char* end) {
    // If this is a one-byte varint, decode it here.
    if (end != *data && ((static_cast<uint64_t>(**data) & 0x80U) == 0)) {
        const auto val = static_cast<uint64_t>(**data);
        ++(*data);
        return val;
    }
    // If this varint is more than one byte, defer to complete implementation.
    return detail::decode_varint_impl(data, end);
}

/**
 * Skip over a varint.
 *
 * Strong exception guarantee: if there is an exception the data pointer will
 * not be changed.
 *
 * @param[in,out] data Pointer to pointer to the input data. After the function
 *        returns this will point to the next data to be read.
 * @param[in] end Pointer one past the end of the input data.
 * @throws end_of_buffer_exception if the *end* of the buffer was reached
 *         before the end of the varint.
 */
inline void skip_varint(const char** data, const char* end) {
    const auto* begin = reinterpret_cast<const int8_t*>(*data);
    const auto* iend = reinterpret_cast<const int8_t*>(end);
    const int8_t* p = begin;

    while (p != iend && *p < 0) {
        ++p;
    }

    if (p - begin >= max_varint_length) {
        throw varint_too_long_exception{};
    }

    if (p == iend) {
        throw end_of_buffer_exception{};
    }

    ++p;

    *data = reinterpret_cast<const char*>(p);
}

/**
 * Varint encode a 64 bit integer.
 *
 * @tparam T An output iterator type.
 * @param data Output iterator the varint encoded value will be written to
 *             byte by byte.
 * @param value The integer that will be encoded.
 * @returns the number of bytes written
 * @throws Any exception thrown by increment or dereference operator on data.
 * @deprecated Use add_varint_to_buffer() instead.
 */
template <typename T>
inline int write_varint(T data, uint64_t value) {
    int n = 1;

    while (value >= 0x80U) {
        *data++ = char((value & 0x7fU) | 0x80U);
        value >>= 7U;
        ++n;
    }
    *data = char(value);

    return n;
}

/**
 * Varint encode a 64 bit integer.
 *
 * @tparam TBuffer A buffer type.
 * @param buffer Output buffer the varint will be written to.
 * @param value The integer that will be encoded.
 * @returns the number of bytes written
 * @throws Any exception thrown by calling the buffer_push_back() function.
 */
template <typename TBuffer>
inline void add_varint_to_buffer(TBuffer* buffer, uint64_t value) {
    while (value >= 0x80U) {
        buffer_customization<TBuffer>::push_back(buffer, char((value & 0x7fU) | 0x80U));
        value >>= 7U;
    }
    buffer_customization<TBuffer>::push_back(buffer, char(value));
}

/**
 * Varint encode a 64 bit integer.
 *
 * @param data Where to add the varint. There must be enough space available!
 * @param value The integer that will be encoded.
 * @returns the number of bytes written
 */
inline int add_varint_to_buffer(char* data, uint64_t value) noexcept {
    int n = 1;

    while (value >= 0x80U) {
        *data++ = char((value & 0x7fU) | 0x80U);
        value >>= 7U;
        ++n;
    }
    *data = char(value);

    return n;
}

/**
 * Get the length of the varint the specified value would produce.
 *
 * @param value The integer to be encoded.
 * @returns the number of bytes the varint would have if we created it.
 */
inline int length_of_varint(uint64_t value) noexcept {
    int n = 1;

    while (value >= 0x80U) {
        value >>= 7U;
        ++n;
    }

    return n;
}

/**
 * ZigZag encodes a 32 bit integer.
 */
inline constexpr uint32_t encode_zigzag32(int32_t value) noexcept {
    return (static_cast<uint32_t>(value) << 1U) ^ static_cast<uint32_t>(-static_cast<int32_t>(static_cast<uint32_t>(value) >> 31U));
}

/**
 * ZigZag encodes a 64 bit integer.
 */
inline constexpr uint64_t encode_zigzag64(int64_t value) noexcept {
    return (static_cast<uint64_t>(value) << 1U) ^ static_cast<uint64_t>(-static_cast<int64_t>(static_cast<uint64_t>(value) >> 63U));
}

/**
 * Decodes a 32 bit ZigZag-encoded integer.
 */
inline constexpr int32_t decode_zigzag32(uint32_t value) noexcept {
    return static_cast<int32_t>((value >> 1U) ^ static_cast<uint32_t>(-static_cast<int32_t>(value & 1U)));
}

/**
 * Decodes a 64 bit ZigZag-encoded integer.
 */
inline constexpr int64_t decode_zigzag64(uint64_t value) noexcept {
    return static_cast<int64_t>((value >> 1U) ^ static_cast<uint64_t>(-static_cast<int64_t>(value & 1U)));
}

} // end namespace protozero

#endif // PROTOZERO_VARINT_HPP
