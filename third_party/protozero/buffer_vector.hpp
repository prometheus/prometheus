#ifndef PROTOZERO_BUFFER_VECTOR_HPP
#define PROTOZERO_BUFFER_VECTOR_HPP

/*****************************************************************************

protozero - Minimalistic protocol buffer decoder and encoder in C++.

This file is from https://github.com/mapbox/protozero where you can find more
documentation.

*****************************************************************************/

/**
 * @file buffer_vector.hpp
 *
 * @brief Contains the customization points for buffer implementation based
 *        on std::vector<char>
 */

#include "buffer_tmpl.hpp"
#include "config.hpp"

#include <cstddef>
#include <iterator>
#include <vector>

namespace protozero {

// Implementation of buffer customizations points for std::vector<char>

/// @cond INTERNAL
template <>
struct buffer_customization<std::vector<char>> {

    static std::size_t size(const std::vector<char>* buffer) noexcept {
        return buffer->size();
    }

    static void append(std::vector<char>* buffer, const char* data, std::size_t count) {
        buffer->insert(buffer->end(), data, data + count);
    }

    static void append_zeros(std::vector<char>* buffer, std::size_t count) {
        buffer->insert(buffer->end(), count, '\0');
    }

    static void resize(std::vector<char>* buffer, std::size_t size) {
        protozero_assert(size < buffer->size());
        buffer->resize(size);
    }

    static void reserve_additional(std::vector<char>* buffer, std::size_t size) {
        buffer->reserve(buffer->size() + size);
    }

    static void erase_range(std::vector<char>* buffer, std::size_t from, std::size_t to) {
        protozero_assert(from <= buffer->size());
        protozero_assert(to <= buffer->size());
        protozero_assert(from <= to);
        buffer->erase(std::next(buffer->begin(), static_cast<std::string::iterator::difference_type>(from)),
                      std::next(buffer->begin(), static_cast<std::string::iterator::difference_type>(to)));
    }

    static char* at_pos(std::vector<char>* buffer, std::size_t pos) {
        protozero_assert(pos <= buffer->size());
        return (&*buffer->begin()) + pos;
    }

    static void push_back(std::vector<char>* buffer, char ch) {
        buffer->push_back(ch);
    }

};
/// @endcond

} // namespace protozero

#endif // PROTOZERO_BUFFER_VECTOR_HPP
