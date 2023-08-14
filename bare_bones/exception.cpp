#include "bare_bones/exception.h"

#include <inttypes.h>  // printf() specifiers for long types.
#include <stdarg.h>

namespace BareBones {
static constexpr size_t user_buffer_msg_size = 255;

static constexpr size_t get_exception_buffer_size() {
  // the HEX representation for type is twice longer than their sizeof() as the 1 byte is written as two HEX digits.
  constexpr size_t byte_size_in_printed_characters = 2;
  // total buffer size must be sufficient for storing the "Exception <HEX-code>: " + user defined message. +1 for \0 (it's got from std::size())
  return user_buffer_msg_size + sizeof(Exception::Code) * byte_size_in_printed_characters + std::size("Exception : ");
}

Exception::Exception(Code exc_code, const char* message, ...) : code_(exc_code) {
  constexpr auto sz = get_exception_buffer_size();
  thread_local char buf[sz];

  buf[sz - 1] = '\0';
  size_t off1 = snprintf(buf, sz, "Exception %" PRIx64 ": ", exc_code);
  va_list list;
  va_start(list, message);

  // Concatenate the user-formatted message (+1 for \0 as snprintf() truncates if formatted string fills the whole buffer)
  size_t l = vsnprintf(buf + off1, user_buffer_msg_size + 1, message, list);
  msg_ = std::string_view(buf, off1 + l + 1);  // +1 for \0
}

}  // namespace BareBones
