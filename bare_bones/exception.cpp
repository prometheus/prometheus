#include "bare_bones/exception.h"

#include <stdarg.h>

namespace BareBones {

Exception::Exception(Code exc_code, const char* message, ...) : code_(exc_code) {
  thread_local char buf[256]; // +1 for '\0'
  buf[255] = '\0';
  va_list list;
  va_start(list, message);
  size_t l = vsnprintf(buf, 255, message, list);
  msg_ = std::string_view(buf, l + 1);
}

}  // namespace BareBones
