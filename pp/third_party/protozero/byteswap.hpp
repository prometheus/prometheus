#ifndef PROTOZERO_BYTESWAP_HPP
#define PROTOZERO_BYTESWAP_HPP

/*****************************************************************************

protozero - Minimalistic protocol buffer decoder and encoder in C++.

This file is from https://github.com/mapbox/protozero where you can find more
documentation.

*****************************************************************************/

/**
 * @file byteswap.hpp
 *
 * @brief Contains functions to swap bytes in values (for different endianness).
 */

#include "config.hpp"

#include <cstdint>
#include <cstring>

namespace protozero {
namespace detail {

inline uint32_t byteswap_impl(uint32_t value) noexcept {
#ifdef PROTOZERO_USE_BUILTIN_BSWAP
    return __builtin_bswap32(value);
#else
    return ((value & 0xff000000U) >> 24U) |
           ((value & 0x00ff0000U) >>  8U) |
           ((value & 0x0000ff00U) <<  8U) |
           ((value & 0x000000ffU) << 24U);
#endif
}

inline uint64_t byteswap_impl(uint64_t value) noexcept {
#ifdef PROTOZERO_USE_BUILTIN_BSWAP
    return __builtin_bswap64(value);
#else
    return ((value & 0xff00000000000000ULL) >> 56U) |
           ((value & 0x00ff000000000000ULL) >> 40U) |
           ((value & 0x0000ff0000000000ULL) >> 24U) |
           ((value & 0x000000ff00000000ULL) >>  8U) |
           ((value & 0x00000000ff000000ULL) <<  8U) |
           ((value & 0x0000000000ff0000ULL) << 24U) |
           ((value & 0x000000000000ff00ULL) << 40U) |
           ((value & 0x00000000000000ffULL) << 56U);
#endif
}

} // end namespace detail

/// byteswap the data pointed to by ptr in-place.
inline void byteswap_inplace(uint32_t* ptr) noexcept {
    *ptr = detail::byteswap_impl(*ptr);
}

/// byteswap the data pointed to by ptr in-place.
inline void byteswap_inplace(uint64_t* ptr) noexcept {
    *ptr = detail::byteswap_impl(*ptr);
}

/// byteswap the data pointed to by ptr in-place.
inline void byteswap_inplace(int32_t* ptr) noexcept {
    auto* bptr = reinterpret_cast<uint32_t*>(ptr);
    *bptr = detail::byteswap_impl(*bptr);
}

/// byteswap the data pointed to by ptr in-place.
inline void byteswap_inplace(int64_t* ptr) noexcept {
    auto* bptr = reinterpret_cast<uint64_t*>(ptr);
    *bptr = detail::byteswap_impl(*bptr);
}

/// byteswap the data pointed to by ptr in-place.
inline void byteswap_inplace(float* ptr) noexcept {
    static_assert(sizeof(float) == 4, "Expecting four byte float");

    uint32_t tmp = 0;
    std::memcpy(&tmp, ptr, 4);
    tmp = detail::byteswap_impl(tmp); // uint32 overload
    std::memcpy(ptr, &tmp, 4);
}

/// byteswap the data pointed to by ptr in-place.
inline void byteswap_inplace(double* ptr) noexcept {
    static_assert(sizeof(double) == 8, "Expecting eight byte double");

    uint64_t tmp = 0;
    std::memcpy(&tmp, ptr, 8);
    tmp = detail::byteswap_impl(tmp); // uint64 overload
    std::memcpy(ptr, &tmp, 8);
}

namespace detail {

    // Added for backwards compatibility with any code that might use this
    // function (even if it shouldn't have). Will be removed in a later
    // version of protozero.
    using ::protozero::byteswap_inplace;

} // end namespace detail

} // end namespace protozero

#endif // PROTOZERO_BYTESWAP_HPP
