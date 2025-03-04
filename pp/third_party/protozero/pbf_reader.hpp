#ifndef PROTOZERO_PBF_READER_HPP
#define PROTOZERO_PBF_READER_HPP

/*****************************************************************************

protozero - Minimalistic protocol buffer decoder and encoder in C++.

This file is from https://github.com/mapbox/protozero where you can find more
documentation.

*****************************************************************************/

/**
 * @file pbf_reader.hpp
 *
 * @brief Contains the pbf_reader class.
 */

#include "config.hpp"
#include "data_view.hpp"
#include "exception.hpp"
#include "iterators.hpp"
#include "types.hpp"
#include "varint.hpp"

#if PROTOZERO_BYTE_ORDER != PROTOZERO_LITTLE_ENDIAN
# include <protozero/byteswap.hpp>
#endif

#include <cstddef>
#include <cstdint>
#include <cstring>
#include <string>
#include <utility>

namespace protozero {

/**
 * This class represents a protobuf message. Either a top-level message or
 * a nested sub-message. Top-level messages can be created from any buffer
 * with a pointer and length:
 *
 * @code
 *    std::string buffer;
 *    // fill buffer...
 *    pbf_reader message{buffer.data(), buffer.size()};
 * @endcode
 *
 * Sub-messages are created using get_message():
 *
 * @code
 *    pbf_reader message{...};
 *    message.next();
 *    pbf_reader submessage = message.get_message();
 * @endcode
 *
 * All methods of the pbf_reader class except get_bytes() and get_string()
 * provide the strong exception guarantee, ie they either succeed or do not
 * change the pbf_reader object they are called on. Use the get_view() method
 * instead of get_bytes() or get_string(), if you need this guarantee.
 */
class pbf_reader {

    // A pointer to the next unread data.
    const char* m_data = nullptr;

    // A pointer to one past the end of data.
    const char* m_end = nullptr;

    // The wire type of the current field.
    pbf_wire_type m_wire_type = pbf_wire_type::unknown;

    // The tag of the current field.
    pbf_tag_type m_tag = 0;

    template <typename T>
    T get_fixed() {
        T result;
        const char* data = m_data;
        skip_bytes(sizeof(T));
        std::memcpy(&result, data, sizeof(T));
#if PROTOZERO_BYTE_ORDER != PROTOZERO_LITTLE_ENDIAN
        byteswap_inplace(&result);
#endif
        return result;
    }

    template <typename T>
    iterator_range<const_fixed_iterator<T>> packed_fixed() {
        protozero_assert(tag() != 0 && "call next() before accessing field value");
        const auto len = get_len_and_skip();
        if (len % sizeof(T) != 0) {
            throw invalid_length_exception{};
        }
        return {const_fixed_iterator<T>(m_data - len),
                const_fixed_iterator<T>(m_data)};
    }

    template <typename T>
    T get_varint() {
        const auto val = static_cast<T>(decode_varint(&m_data, m_end));
        return val;
    }

    template <typename T>
    T get_svarint() {
        protozero_assert((has_wire_type(pbf_wire_type::varint) || has_wire_type(pbf_wire_type::length_delimited)) && "not a varint");
        return static_cast<T>(decode_zigzag64(decode_varint(&m_data, m_end)));
    }

    pbf_length_type get_length() {
        return get_varint<pbf_length_type>();
    }

    void skip_bytes(pbf_length_type len) {
        if (m_end - m_data < static_cast<ptrdiff_t>(len)) {
            throw end_of_buffer_exception{};
        }
        m_data += len;

#ifndef NDEBUG
        // In debug builds reset the tag to zero so that we can detect (some)
        // wrong code.
        m_tag = 0;
#endif
    }

    pbf_length_type get_len_and_skip() {
        const auto len = get_length();
        skip_bytes(len);
        return len;
    }

    template <typename T>
    iterator_range<T> get_packed() {
        protozero_assert(tag() != 0 && "call next() before accessing field value");
        const auto len = get_len_and_skip();
        return {T{m_data - len, m_data},
                T{m_data, m_data}};
    }

public:

    /**
     * Construct a pbf_reader message from a data_view. The pointer from the
     * data_view will be stored inside the pbf_reader object, no data is
     * copied. So you must make sure the view stays valid as long as the
     * pbf_reader object is used.
     *
     * The buffer must contain a complete protobuf message.
     *
     * @post There is no current field.
     */
    explicit pbf_reader(const data_view& view) noexcept
        : m_data{view.data()},
          m_end{view.data() + view.size()} {
    }

    /**
     * Construct a pbf_reader message from a data pointer and a length. The
     * pointer will be stored inside the pbf_reader object, no data is copied.
     * So you must make sure the buffer stays valid as long as the pbf_reader
     * object is used.
     *
     * The buffer must contain a complete protobuf message.
     *
     * @post There is no current field.
     */
    pbf_reader(const char* data, std::size_t size) noexcept
        : m_data{data},
          m_end{data + size} {
    }

#ifndef PROTOZERO_STRICT_API
    /**
     * Construct a pbf_reader message from a data pointer and a length. The
     * pointer will be stored inside the pbf_reader object, no data is copied.
     * So you must make sure the buffer stays valid as long as the pbf_reader
     * object is used.
     *
     * The buffer must contain a complete protobuf message.
     *
     * @post There is no current field.
     * @deprecated Use one of the other constructors.
     */
    explicit pbf_reader(const std::pair<const char*, std::size_t>& data) noexcept
        : m_data{data.first},
          m_end{data.first + data.second} {
    }
#endif

    /**
     * Construct a pbf_reader message from a std::string. A pointer to the
     * string internals will be stored inside the pbf_reader object, no data
     * is copied. So you must make sure the string is unchanged as long as the
     * pbf_reader object is used.
     *
     * The string must contain a complete protobuf message.
     *
     * @post There is no current field.
     */
    explicit pbf_reader(const std::string& data) noexcept
        : m_data{data.data()},
          m_end{data.data() + data.size()} {
    }

    /**
     * pbf_reader can be default constructed and behaves like it has an empty
     * buffer.
     */
    pbf_reader() noexcept = default;

    /// pbf_reader messages can be copied trivially.
    pbf_reader(const pbf_reader&) noexcept = default;

    /// pbf_reader messages can be moved trivially.
    pbf_reader(pbf_reader&&) noexcept = default;

    /// pbf_reader messages can be copied trivially.
    pbf_reader& operator=(const pbf_reader& other) noexcept = default;

    /// pbf_reader messages can be moved trivially.
    pbf_reader& operator=(pbf_reader&& other) noexcept = default;

    ~pbf_reader() = default;

    /**
     * Swap the contents of this object with the other.
     *
     * @param other Other object to swap data with.
     */
    void swap(pbf_reader& other) noexcept {
        using std::swap;
        swap(m_data, other.m_data);
        swap(m_end, other.m_end);
        swap(m_wire_type, other.m_wire_type);
        swap(m_tag, other.m_tag);
    }

    /**
     * In a boolean context the pbf_reader class evaluates to `true` if there
     * are still fields available and to `false` if the last field has been
     * read.
     */
    operator bool() const noexcept { // NOLINT(google-explicit-constructor, hicpp-explicit-conversions)
        return m_data != m_end;
    }

    /**
     * Get a view of the not yet read data.
     */
    data_view data() const noexcept {
        return {m_data, static_cast<std::size_t>(m_end - m_data)};
    }

    /**
     * Return the length in bytes of the current message. If you have
     * already called next() and/or any of the get_*() functions, this will
     * return the remaining length.
     *
     * This can, for instance, be used to estimate the space needed for a
     * buffer. Of course you have to know reasonably well what data to expect
     * and how it is encoded for this number to have any meaning.
     */
    std::size_t length() const noexcept {
        return std::size_t(m_end - m_data);
    }

    /**
     * Set next field in the message as the current field. This is usually
     * called in a while loop:
     *
     * @code
     *    pbf_reader message(...);
     *    while (message.next()) {
     *        // handle field
     *    }
     * @endcode
     *
     * @returns `true` if there is a next field, `false` if not.
     * @pre There must be no current field.
     * @post If it returns `true` there is a current field now.
     */
    bool next() {
        if (m_data == m_end) {
            return false;
        }

        const auto value = get_varint<uint32_t>();
        m_tag = pbf_tag_type(value >> 3U);

        // tags 0 and 19000 to 19999 are not allowed as per
        // https://developers.google.com/protocol-buffers/docs/proto#assigning-tags
        if (m_tag == 0 || (m_tag >= 19000 && m_tag <= 19999)) {
            throw invalid_tag_exception{};
        }

        m_wire_type = pbf_wire_type(value & 0x07U);
        switch (m_wire_type) {
            case pbf_wire_type::varint:
            case pbf_wire_type::fixed64:
            case pbf_wire_type::length_delimited:
            case pbf_wire_type::fixed32:
                break;
            default:
                throw unknown_pbf_wire_type_exception{};
        }

        return true;
    }

    /**
     * Set next field with given tag in the message as the current field.
     * Fields with other tags are skipped. This is usually called in a while
     * loop for repeated fields:
     *
     * @code
     *    pbf_reader message{...};
     *    while (message.next(17)) {
     *        // handle field
     *    }
     * @endcode
     *
     * or you can call it just once to get the one field with this tag:
     *
     * @code
     *    pbf_reader message{...};
     *    if (message.next(17)) {
     *        // handle field
     *    }
     * @endcode
     *
     * Note that this will not check the wire type. The two-argument version
     * of this function will also check the wire type.
     *
     * @returns `true` if there is a next field with this tag.
     * @pre There must be no current field.
     * @post If it returns `true` there is a current field now with the given tag.
     */
    bool next(pbf_tag_type next_tag) {
        while (next()) {
            if (m_tag == next_tag) {
                return true;
            }
            skip();
        }
        return false;
    }

    /**
     * Set next field with given tag and wire type in the message as the
     * current field. Fields with other tags are skipped. This is usually
     * called in a while loop for repeated fields:
     *
     * @code
     *    pbf_reader message{...};
     *    while (message.next(17, pbf_wire_type::varint)) {
     *        // handle field
     *    }
     * @endcode
     *
     * or you can call it just once to get the one field with this tag:
     *
     * @code
     *    pbf_reader message{...};
     *    if (message.next(17, pbf_wire_type::varint)) {
     *        // handle field
     *    }
     * @endcode
     *
     * Note that this will also check the wire type. The one-argument version
     * of this function will not check the wire type.
     *
     * @returns `true` if there is a next field with this tag.
     * @pre There must be no current field.
     * @post If it returns `true` there is a current field now with the given tag.
     */
    bool next(pbf_tag_type next_tag, pbf_wire_type type) {
        while (next()) {
            if (m_tag == next_tag && m_wire_type == type) {
                return true;
            }
            skip();
        }
        return false;
    }

    /**
     * The tag of the current field. The tag is the field number from the
     * description in the .proto file.
     *
     * Call next() before calling this function to set the current field.
     *
     * @returns tag of the current field.
     * @pre There must be a current field (ie. next() must have returned `true`).
     */
    pbf_tag_type tag() const noexcept {
        return m_tag;
    }

    /**
     * Get the wire type of the current field. The wire types are:
     *
     * * 0 - varint
     * * 1 - 64 bit
     * * 2 - length-delimited
     * * 5 - 32 bit
     *
     * All other types are illegal.
     *
     * Call next() before calling this function to set the current field.
     *
     * @returns wire type of the current field.
     * @pre There must be a current field (ie. next() must have returned `true`).
     */
    pbf_wire_type wire_type() const noexcept {
        return m_wire_type;
    }

    /**
     * Get the tag and wire type of the current field in one integer suitable
     * for comparison with a switch statement.
     *
     * Use it like this:
     *
     * @code
     *    pbf_reader message{...};
     *    while (message.next()) {
     *        switch (message.tag_and_type()) {
     *            case tag_and_type(17, pbf_wire_type::length_delimited):
     *                ....
     *                break;
     *            case tag_and_type(21, pbf_wire_type::varint):
     *                ....
     *                break;
     *            default:
     *                message.skip();
     *        }
     *    }
     * @endcode
     */
    uint32_t tag_and_type() const noexcept {
        return protozero::tag_and_type(tag(), wire_type());
    }

    /**
     * Check the wire type of the current field.
     *
     * @returns `true` if the current field has the given wire type.
     * @pre There must be a current field (ie. next() must have returned `true`).
     */
    bool has_wire_type(pbf_wire_type type) const noexcept {
        return wire_type() == type;
    }

    /**
     * Consume the current field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @post The current field was consumed and there is no current field now.
     */
    void skip() {
        protozero_assert(tag() != 0 && "call next() before calling skip()");
        switch (wire_type()) {
            case pbf_wire_type::varint:
                skip_varint(&m_data, m_end);
                break;
            case pbf_wire_type::fixed64:
                skip_bytes(8);
                break;
            case pbf_wire_type::length_delimited:
                skip_bytes(get_length());
                break;
            case pbf_wire_type::fixed32:
                skip_bytes(4);
                break;
            default:
                break;
        }
    }

    ///@{
    /**
     * @name Scalar field accessor functions
     */

    /**
     * Consume and return value of current "bool" field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "bool".
     * @post The current field was consumed and there is no current field now.
     */
    bool get_bool() {
        protozero_assert(tag() != 0 && "call next() before accessing field value");
        protozero_assert(has_wire_type(pbf_wire_type::varint) && "not a varint");
        const bool result = m_data[0] != 0;
        skip_varint(&m_data, m_end);
        return result;
    }

    /**
     * Consume and return value of current "enum" field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "enum".
     * @post The current field was consumed and there is no current field now.
     */
    int32_t get_enum() {
        protozero_assert(has_wire_type(pbf_wire_type::varint) && "not a varint");
        return get_varint<int32_t>();
    }

    /**
     * Consume and return value of current "int32" varint field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "int32".
     * @post The current field was consumed and there is no current field now.
     */
    int32_t get_int32() {
        protozero_assert(has_wire_type(pbf_wire_type::varint) && "not a varint");
        return get_varint<int32_t>();
    }

    /**
     * Consume and return value of current "sint32" varint field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "sint32".
     * @post The current field was consumed and there is no current field now.
     */
    int32_t get_sint32() {
        protozero_assert(has_wire_type(pbf_wire_type::varint) && "not a varint");
        return get_svarint<int32_t>();
    }

    /**
     * Consume and return value of current "uint32" varint field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "uint32".
     * @post The current field was consumed and there is no current field now.
     */
    uint32_t get_uint32() {
        protozero_assert(has_wire_type(pbf_wire_type::varint) && "not a varint");
        return get_varint<uint32_t>();
    }

    /**
     * Consume and return value of current "int64" varint field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "int64".
     * @post The current field was consumed and there is no current field now.
     */
    int64_t get_int64() {
        protozero_assert(has_wire_type(pbf_wire_type::varint) && "not a varint");
        return get_varint<int64_t>();
    }

    /**
     * Consume and return value of current "sint64" varint field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "sint64".
     * @post The current field was consumed and there is no current field now.
     */
    int64_t get_sint64() {
        protozero_assert(has_wire_type(pbf_wire_type::varint) && "not a varint");
        return get_svarint<int64_t>();
    }

    /**
     * Consume and return value of current "uint64" varint field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "uint64".
     * @post The current field was consumed and there is no current field now.
     */
    uint64_t get_uint64() {
        protozero_assert(has_wire_type(pbf_wire_type::varint) && "not a varint");
        return get_varint<uint64_t>();
    }

    /**
     * Consume and return value of current "fixed32" field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "fixed32".
     * @post The current field was consumed and there is no current field now.
     */
    uint32_t get_fixed32() {
        protozero_assert(tag() != 0 && "call next() before accessing field value");
        protozero_assert(has_wire_type(pbf_wire_type::fixed32) && "not a 32-bit fixed");
        return get_fixed<uint32_t>();
    }

    /**
     * Consume and return value of current "sfixed32" field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "sfixed32".
     * @post The current field was consumed and there is no current field now.
     */
    int32_t get_sfixed32() {
        protozero_assert(tag() != 0 && "call next() before accessing field value");
        protozero_assert(has_wire_type(pbf_wire_type::fixed32) && "not a 32-bit fixed");
        return get_fixed<int32_t>();
    }

    /**
     * Consume and return value of current "fixed64" field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "fixed64".
     * @post The current field was consumed and there is no current field now.
     */
    uint64_t get_fixed64() {
        protozero_assert(tag() != 0 && "call next() before accessing field value");
        protozero_assert(has_wire_type(pbf_wire_type::fixed64) && "not a 64-bit fixed");
        return get_fixed<uint64_t>();
    }

    /**
     * Consume and return value of current "sfixed64" field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "sfixed64".
     * @post The current field was consumed and there is no current field now.
     */
    int64_t get_sfixed64() {
        protozero_assert(tag() != 0 && "call next() before accessing field value");
        protozero_assert(has_wire_type(pbf_wire_type::fixed64) && "not a 64-bit fixed");
        return get_fixed<int64_t>();
    }

    /**
     * Consume and return value of current "float" field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "float".
     * @post The current field was consumed and there is no current field now.
     */
    float get_float() {
        protozero_assert(tag() != 0 && "call next() before accessing field value");
        protozero_assert(has_wire_type(pbf_wire_type::fixed32) && "not a 32-bit fixed");
        return get_fixed<float>();
    }

    /**
     * Consume and return value of current "double" field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "double".
     * @post The current field was consumed and there is no current field now.
     */
    double get_double() {
        protozero_assert(tag() != 0 && "call next() before accessing field value");
        protozero_assert(has_wire_type(pbf_wire_type::fixed64) && "not a 64-bit fixed");
        return get_fixed<double>();
    }

    /**
     * Consume and return value of current "bytes", "string", or "message"
     * field.
     *
     * @returns A data_view object.
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "bytes", "string", or "message".
     * @post The current field was consumed and there is no current field now.
     */
    data_view get_view() {
        protozero_assert(tag() != 0 && "call next() before accessing field value");
        protozero_assert(has_wire_type(pbf_wire_type::length_delimited) && "not of type string, bytes or message");
        const auto len = get_len_and_skip();
        return {m_data - len, len};
    }

#ifndef PROTOZERO_STRICT_API
    /**
     * Consume and return value of current "bytes" or "string" field.
     *
     * @returns A pair with a pointer to the data and the length of the data.
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "bytes" or "string".
     * @post The current field was consumed and there is no current field now.
     */
    std::pair<const char*, pbf_length_type> get_data() {
        protozero_assert(tag() != 0 && "call next() before accessing field value");
        protozero_assert(has_wire_type(pbf_wire_type::length_delimited) && "not of type string, bytes or message");
        const auto len = get_len_and_skip();
        return {m_data - len, len};
    }
#endif

    /**
     * Consume and return value of current "bytes" field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "bytes".
     * @post The current field was consumed and there is no current field now.
     */
    std::string get_bytes() {
        return std::string(get_view());
    }

    /**
     * Consume and return value of current "string" field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "string".
     * @post The current field was consumed and there is no current field now.
     */
    std::string get_string() {
        return std::string(get_view());
    }

    /**
     * Consume and return value of current "message" field.
     *
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "message".
     * @post The current field was consumed and there is no current field now.
     */
    pbf_reader get_message() {
        return pbf_reader{get_view()};
    }

    ///@}

    /// Forward iterator for iterating over bool (int32 varint) values.
    using const_bool_iterator   = const_varint_iterator< int32_t>;

    /// Forward iterator for iterating over enum (int32 varint) values.
    using const_enum_iterator   = const_varint_iterator< int32_t>;

    /// Forward iterator for iterating over int32 (varint) values.
    using const_int32_iterator  = const_varint_iterator< int32_t>;

    /// Forward iterator for iterating over sint32 (varint) values.
    using const_sint32_iterator = const_svarint_iterator<int32_t>;

    /// Forward iterator for iterating over uint32 (varint) values.
    using const_uint32_iterator = const_varint_iterator<uint32_t>;

    /// Forward iterator for iterating over int64 (varint) values.
    using const_int64_iterator  = const_varint_iterator< int64_t>;

    /// Forward iterator for iterating over sint64 (varint) values.
    using const_sint64_iterator = const_svarint_iterator<int64_t>;

    /// Forward iterator for iterating over uint64 (varint) values.
    using const_uint64_iterator = const_varint_iterator<uint64_t>;

    /// Forward iterator for iterating over fixed32 values.
    using const_fixed32_iterator = const_fixed_iterator<uint32_t>;

    /// Forward iterator for iterating over sfixed32 values.
    using const_sfixed32_iterator = const_fixed_iterator<int32_t>;

    /// Forward iterator for iterating over fixed64 values.
    using const_fixed64_iterator = const_fixed_iterator<uint64_t>;

    /// Forward iterator for iterating over sfixed64 values.
    using const_sfixed64_iterator = const_fixed_iterator<int64_t>;

    /// Forward iterator for iterating over float values.
    using const_float_iterator = const_fixed_iterator<float>;

    /// Forward iterator for iterating over double values.
    using const_double_iterator = const_fixed_iterator<double>;

    ///@{
    /**
     * @name Repeated packed field accessor functions
     */

    /**
     * Consume current "repeated packed bool" field.
     *
     * @returns a pair of iterators to the beginning and one past the end of
     *          the data.
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "repeated packed bool".
     * @post The current field was consumed and there is no current field now.
     */
    iterator_range<pbf_reader::const_bool_iterator> get_packed_bool() {
        return get_packed<pbf_reader::const_bool_iterator>();
    }

    /**
     * Consume current "repeated packed enum" field.
     *
     * @returns a pair of iterators to the beginning and one past the end of
     *          the data.
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "repeated packed enum".
     * @post The current field was consumed and there is no current field now.
     */
    iterator_range<pbf_reader::const_enum_iterator> get_packed_enum() {
        return get_packed<pbf_reader::const_enum_iterator>();
    }

    /**
     * Consume current "repeated packed int32" field.
     *
     * @returns a pair of iterators to the beginning and one past the end of
     *          the data.
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "repeated packed int32".
     * @post The current field was consumed and there is no current field now.
     */
    iterator_range<pbf_reader::const_int32_iterator> get_packed_int32() {
        return get_packed<pbf_reader::const_int32_iterator>();
    }

    /**
     * Consume current "repeated packed sint32" field.
     *
     * @returns a pair of iterators to the beginning and one past the end of
     *          the data.
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "repeated packed sint32".
     * @post The current field was consumed and there is no current field now.
     */
    iterator_range<pbf_reader::const_sint32_iterator> get_packed_sint32() {
        return get_packed<pbf_reader::const_sint32_iterator>();
    }

    /**
     * Consume current "repeated packed uint32" field.
     *
     * @returns a pair of iterators to the beginning and one past the end of
     *          the data.
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "repeated packed uint32".
     * @post The current field was consumed and there is no current field now.
     */
    iterator_range<pbf_reader::const_uint32_iterator> get_packed_uint32() {
        return get_packed<pbf_reader::const_uint32_iterator>();
    }

    /**
     * Consume current "repeated packed int64" field.
     *
     * @returns a pair of iterators to the beginning and one past the end of
     *          the data.
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "repeated packed int64".
     * @post The current field was consumed and there is no current field now.
     */
    iterator_range<pbf_reader::const_int64_iterator> get_packed_int64() {
        return get_packed<pbf_reader::const_int64_iterator>();
    }

    /**
     * Consume current "repeated packed sint64" field.
     *
     * @returns a pair of iterators to the beginning and one past the end of
     *          the data.
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "repeated packed sint64".
     * @post The current field was consumed and there is no current field now.
     */
    iterator_range<pbf_reader::const_sint64_iterator> get_packed_sint64() {
        return get_packed<pbf_reader::const_sint64_iterator>();
    }

    /**
     * Consume current "repeated packed uint64" field.
     *
     * @returns a pair of iterators to the beginning and one past the end of
     *          the data.
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "repeated packed uint64".
     * @post The current field was consumed and there is no current field now.
     */
    iterator_range<pbf_reader::const_uint64_iterator> get_packed_uint64() {
        return get_packed<pbf_reader::const_uint64_iterator>();
    }

    /**
     * Consume current "repeated packed fixed32" field.
     *
     * @returns a pair of iterators to the beginning and one past the end of
     *          the data.
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "repeated packed fixed32".
     * @post The current field was consumed and there is no current field now.
     */
    iterator_range<pbf_reader::const_fixed32_iterator> get_packed_fixed32() {
        return packed_fixed<uint32_t>();
    }

    /**
     * Consume current "repeated packed sfixed32" field.
     *
     * @returns a pair of iterators to the beginning and one past the end of
     *          the data.
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "repeated packed sfixed32".
     * @post The current field was consumed and there is no current field now.
     */
    iterator_range<pbf_reader::const_sfixed32_iterator> get_packed_sfixed32() {
        return packed_fixed<int32_t>();
    }

    /**
     * Consume current "repeated packed fixed64" field.
     *
     * @returns a pair of iterators to the beginning and one past the end of
     *          the data.
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "repeated packed fixed64".
     * @post The current field was consumed and there is no current field now.
     */
    iterator_range<pbf_reader::const_fixed64_iterator> get_packed_fixed64() {
        return packed_fixed<uint64_t>();
    }

    /**
     * Consume current "repeated packed sfixed64" field.
     *
     * @returns a pair of iterators to the beginning and one past the end of
     *          the data.
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "repeated packed sfixed64".
     * @post The current field was consumed and there is no current field now.
     */
    iterator_range<pbf_reader::const_sfixed64_iterator> get_packed_sfixed64() {
        return packed_fixed<int64_t>();
    }

    /**
     * Consume current "repeated packed float" field.
     *
     * @returns a pair of iterators to the beginning and one past the end of
     *          the data.
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "repeated packed float".
     * @post The current field was consumed and there is no current field now.
     */
    iterator_range<pbf_reader::const_float_iterator> get_packed_float() {
        return packed_fixed<float>();
    }

    /**
     * Consume current "repeated packed double" field.
     *
     * @returns a pair of iterators to the beginning and one past the end of
     *          the data.
     * @pre There must be a current field (ie. next() must have returned `true`).
     * @pre The current field must be of type "repeated packed double".
     * @post The current field was consumed and there is no current field now.
     */
    iterator_range<pbf_reader::const_double_iterator> get_packed_double() {
        return packed_fixed<double>();
    }

    ///@}

}; // class pbf_reader

/**
 * Swap two pbf_reader objects.
 *
 * @param lhs First object.
 * @param rhs Second object.
 */
inline void swap(pbf_reader& lhs, pbf_reader& rhs) noexcept {
    lhs.swap(rhs);
}

} // end namespace protozero

#endif // PROTOZERO_PBF_READER_HPP
