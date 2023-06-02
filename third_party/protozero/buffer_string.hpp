#ifndef PROTOZERO_BUFFER_STRING_HPP
#define PROTOZERO_BUFFER_STRING_HPP

/*****************************************************************************

protozero - Minimalistic protocol buffer decoder and encoder in C++.

This file is from https://github.com/mapbox/protozero where you can find more
documentation.

*****************************************************************************/

/**
 * @file buffer_string.hpp
 *
 * @brief Contains the customization points for buffer implementation based
 *        on std::string
 */

#include "buffer_tmpl.hpp"
#include "config.hpp"

#include <cstddef>
#include <iterator>
#include <string>

namespace protozero {

// Implementation of buffer customizations points for std::string

/// @cond INTERNAL
template <>
struct buffer_customization<std::string> {

    static std::size_t size(const std::string* buffer) noexcept {
        return buffer->size();
    }

    static void append(std::string* buffer, const char* data, std::size_t count) {
        buffer->append(data, count);
    }

    static void append_zeros(std::string* buffer, std::size_t count) {
        buffer->append(count, '\0');
    }

    static void resize(std::string* buffer, std::size_t size) {
        protozero_assert(size < buffer->size());
        buffer->resize(size);
    }

    static void reserve_additional(std::string* buffer, std::size_t size) {
        buffer->reserve(buffer->size() + size);
    }

    static void erase_range(std::string* buffer, std::size_t from, std::size_t to) {
        protozero_assert(from <= buffer->size());
        protozero_assert(to <= buffer->size());
        protozero_assert(from <= to);
        buffer->erase(std::next(buffer->begin(), static_cast<std::string::iterator::difference_type>(from)),
                      std::next(buffer->begin(), static_cast<std::string::iterator::difference_type>(to)));
    }

    static char* at_pos(std::string* buffer, std::size_t pos) {
        protozero_assert(pos <= buffer->size());
        return (&*buffer->begin()) + pos;
    }

    static void push_back(std::string* buffer, char ch) {
        buffer->push_back(ch);
    }

};
/// @endcond

} // namespace protozero

#endif // PROTOZERO_BUFFER_STRING_HPP
