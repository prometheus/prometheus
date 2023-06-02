#ifndef PROTOZERO_DATA_VIEW_HPP
#define PROTOZERO_DATA_VIEW_HPP

/*****************************************************************************

protozero - Minimalistic protocol buffer decoder and encoder in C++.

This file is from https://github.com/mapbox/protozero where you can find more
documentation.

*****************************************************************************/

/**
 * @file data_view.hpp
 *
 * @brief Contains the implementation of the data_view class.
 */

#include "config.hpp"

#include <algorithm>
#include <cstddef>
#include <cstring>
#include <string>
#include <utility>

namespace protozero {

#ifdef PROTOZERO_USE_VIEW
using data_view = PROTOZERO_USE_VIEW;
#else

/**
 * Holds a pointer to some data and a length.
 *
 * This class is supposed to be compatible with the std::string_view
 * that will be available in C++17.
 */
class data_view {

    const char* m_data = nullptr;
    std::size_t m_size = 0;

public:

    /**
     * Default constructor. Construct an empty data_view.
     */
    constexpr data_view() noexcept = default;

    /**
     * Create data_view from pointer and size.
     *
     * @param ptr Pointer to the data.
     * @param length Length of the data.
     */
    constexpr data_view(const char* ptr, std::size_t length) noexcept
        : m_data{ptr},
          m_size{length} {
    }

    /**
     * Create data_view from string.
     *
     * @param str String with the data.
     */
    data_view(const std::string& str) noexcept // NOLINT(google-explicit-constructor, hicpp-explicit-conversions)
        : m_data{str.data()},
          m_size{str.size()} {
    }

    /**
     * Create data_view from zero-terminated string.
     *
     * @param ptr Pointer to the data.
     */
    data_view(const char* ptr) noexcept // NOLINT(google-explicit-constructor, hicpp-explicit-conversions)
        : m_data{ptr},
          m_size{std::strlen(ptr)} {
    }

    /**
     * Swap the contents of this object with the other.
     *
     * @param other Other object to swap data with.
     */
    void swap(data_view& other) noexcept {
        using std::swap;
        swap(m_data, other.m_data);
        swap(m_size, other.m_size);
    }

    /// Return pointer to data.
    constexpr const char* data() const noexcept {
        return m_data;
    }

    /// Return length of data in bytes.
    constexpr std::size_t size() const noexcept {
        return m_size;
    }

    /// Returns true if size is 0.
    constexpr bool empty() const noexcept {
        return m_size == 0;
    }

#ifndef PROTOZERO_STRICT_API
    /**
     * Convert data view to string.
     *
     * @pre Must not be default constructed data_view.
     *
     * @deprecated to_string() is not available in C++17 string_view so it
     *             should not be used to make conversion to that class easier
     *             in the future.
     */
    std::string to_string() const {
        protozero_assert(m_data);
        return {m_data, m_size};
    }
#endif

    /**
     * Convert data view to string.
     *
     * @pre Must not be default constructed data_view.
     */
    explicit operator std::string() const {
        protozero_assert(m_data);
        return {m_data, m_size};
    }

    /**
     * Compares the contents of this object with the given other object.
     *
     * @returns 0 if they are the same, <0 if this object is smaller than
     *          the other or >0 if it is larger. If both objects have the
     *          same size returns <0 if this object is lexicographically
     *          before the other, >0 otherwise.
     *
     * @pre Must not be default constructed data_view.
     */
    int compare(data_view other) const noexcept {
        assert(m_data && other.m_data);
        const int cmp = std::memcmp(data(), other.data(),
                                    std::min(size(), other.size()));
        if (cmp == 0) {
            if (size() == other.size()) {
                return 0;
            }
            return size() < other.size() ? -1 : 1;
        }
        return cmp;
    }

}; // class data_view

/**
 * Swap two data_view objects.
 *
 * @param lhs First object.
 * @param rhs Second object.
 */
inline void swap(data_view& lhs, data_view& rhs) noexcept {
    lhs.swap(rhs);
}

/**
 * Two data_view instances are equal if they have the same size and the
 * same content.
 *
 * @param lhs First object.
 * @param rhs Second object.
 */
inline constexpr bool operator==(const data_view lhs, const data_view rhs) noexcept {
    return lhs.size() == rhs.size() &&
           std::equal(lhs.data(), lhs.data() + lhs.size(), rhs.data());
}

/**
 * Two data_view instances are not equal if they have different sizes or the
 * content differs.
 *
 * @param lhs First object.
 * @param rhs Second object.
 */
inline constexpr bool operator!=(const data_view lhs, const data_view rhs) noexcept {
    return !(lhs == rhs);
}

/**
 * Returns true if lhs.compare(rhs) < 0.
 *
 * @param lhs First object.
 * @param rhs Second object.
 */
inline bool operator<(const data_view lhs, const data_view rhs) noexcept {
    return lhs.compare(rhs) < 0;
}

/**
 * Returns true if lhs.compare(rhs) <= 0.
 *
 * @param lhs First object.
 * @param rhs Second object.
 */
inline bool operator<=(const data_view lhs, const data_view rhs) noexcept {
    return lhs.compare(rhs) <= 0;
}

/**
 * Returns true if lhs.compare(rhs) > 0.
 *
 * @param lhs First object.
 * @param rhs Second object.
 */
inline bool operator>(const data_view lhs, const data_view rhs) noexcept {
    return lhs.compare(rhs) > 0;
}

/**
 * Returns true if lhs.compare(rhs) >= 0.
 *
 * @param lhs First object.
 * @param rhs Second object.
 */
inline bool operator>=(const data_view lhs, const data_view rhs) noexcept {
    return lhs.compare(rhs) >= 0;
}

#endif

} // end namespace protozero

#endif // PROTOZERO_DATA_VIEW_HPP
