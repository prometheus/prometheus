#ifndef PROTOZERO_EXCEPTION_HPP
#define PROTOZERO_EXCEPTION_HPP

/*****************************************************************************

protozero - Minimalistic protocol buffer decoder and encoder in C++.

This file is from https://github.com/mapbox/protozero where you can find more
documentation.

*****************************************************************************/

/**
 * @file exception.hpp
 *
 * @brief Contains the exceptions used in the protozero library.
 */

#include <exception>

/**
 * @brief All parts of the protozero header-only library are in this namespace.
 */
namespace protozero {

/**
 * All exceptions explicitly thrown by the functions of the protozero library
 * derive from this exception.
 */
struct exception : std::exception {
    /// Returns the explanatory string.
    const char* what() const noexcept override {
        return "pbf exception";
    }
};

/**
 * This exception is thrown when parsing a varint thats larger than allowed.
 * This should never happen unless the data is corrupted.
 */
struct varint_too_long_exception : exception {
    /// Returns the explanatory string.
    const char* what() const noexcept override {
        return "varint too long exception";
    }
};

/**
 * This exception is thrown when the wire type of a pdf field is unknown.
 * This should never happen unless the data is corrupted.
 */
struct unknown_pbf_wire_type_exception : exception {
    /// Returns the explanatory string.
    const char* what() const noexcept override {
        return "unknown pbf field type exception";
    }
};

/**
 * This exception is thrown when we are trying to read a field and there
 * are not enough bytes left in the buffer to read it. Almost all functions
 * of the pbf_reader class can throw this exception.
 *
 * This should never happen unless the data is corrupted or you have
 * initialized the pbf_reader object with incomplete data.
 */
struct end_of_buffer_exception : exception {
    /// Returns the explanatory string.
    const char* what() const noexcept override {
        return "end of buffer exception";
    }
};

/**
 * This exception is thrown when a tag has an invalid value. Tags must be
 * unsigned integers between 1 and 2^29-1. Tags between 19000 and 19999 are
 * not allowed. See
 * https://developers.google.com/protocol-buffers/docs/proto#assigning-tags
 */
struct invalid_tag_exception : exception {
    /// Returns the explanatory string.
    const char* what() const noexcept override {
        return "invalid tag exception";
    }
};

/**
 * This exception is thrown when a length field of a packed repeated field is
 * invalid. For fixed size types the length must be a multiple of the size of
 * the type.
 */
struct invalid_length_exception : exception {
    /// Returns the explanatory string.
    const char* what() const noexcept override {
        return "invalid length exception";
    }
};

} // end namespace protozero

#endif // PROTOZERO_EXCEPTION_HPP
