#ifndef PROTOZERO_BUFFER_TMPL_HPP
#define PROTOZERO_BUFFER_TMPL_HPP

/*****************************************************************************

protozero - Minimalistic protocol buffer decoder and encoder in C++.

This file is from https://github.com/mapbox/protozero where you can find more
documentation.

*****************************************************************************/

/**
 * @file buffer_tmpl.hpp
 *
 * @brief Contains the customization points for buffer implementations.
 */

#include <cstddef>
#include <iterator>
#include <string>

namespace protozero {

// Implementation of buffer customizations points for std::string

/// @cond INTERNAL
template <typename T>
struct buffer_customization {

    /**
     * Get the number of bytes currently used in the buffer.
     *
     * @param buffer Pointer to the buffer.
     * @returns number of bytes used in the buffer.
     */
    static std::size_t size(const std::string* buffer);

    /**
     * Append count bytes from data to the buffer.
     *
     * @param buffer Pointer to the buffer.
     * @param data Pointer to the data.
     * @param count Number of bytes to be added to the buffer.
     */
    static void append(std::string* buffer, const char* data, std::size_t count);

    /**
     * Append count zero bytes to the buffer.
     *
     * @param buffer Pointer to the buffer.
     * @param count Number of bytes to be added to the buffer.
     */
    static void append_zeros(std::string* buffer, std::size_t count);

    /**
     * Shrink the buffer to the specified size. The new size will always be
     * smaller than the current size.
     *
     * @param buffer Pointer to the buffer.
     * @param size New size of the buffer.
     *
     * @pre size < current size of buffer
     */
    static void resize(std::string* buffer, std::size_t size);

    /**
     * Reserve an additional size bytes for use in the buffer. This is used for
     * variable-sized buffers to tell the buffer implementation that soon more
     * memory will be used. The implementation can ignore this.
     *
     * @param buffer Pointer to the buffer.
     * @param size Number of bytes to reserve.
     */
    static void reserve_additional(std::string* buffer, std::size_t size);

    /**
     * Delete data from the buffer. This must move back the data after the
     * part being deleted and resize the buffer accordingly.
     *
     * @param buffer Pointer to the buffer.
     * @param from Offset into the buffer where we want to erase from.
     * @param to Offset into the buffer one past the last byte we want to erase.
     *
     * @pre from, to <= size of the buffer, from < to
     */
    static void erase_range(std::string* buffer, std::size_t from, std::size_t to);

    /**
     * Return a pointer to the memory at the specified position in the buffer.
     *
     * @param buffer Pointer to the buffer.
     * @param pos The position in the buffer.
     * @returns pointer to the memory in the buffer at the specified position.
     *
     * @pre pos <= size of the buffer
     */
    static char* at_pos(std::string* buffer, std::size_t pos);

    /**
     * Add a char to the buffer incrementing the number of chars in the buffer.
     *
     * @param buffer Pointer to the buffer.
     * @param ch The character to add.
     */
    static void push_back(std::string* buffer, char ch);

};
/// @endcond

} // namespace protozero

#endif // PROTOZERO_BUFFER_TMPL_HPP
