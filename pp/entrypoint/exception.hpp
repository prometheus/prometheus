#pragma once

#include <ostream>
#include <source_location>
#include <string_view>

#include "bare_bones/exception.h"
#include "bare_bones/preprocess.h"

namespace entrypoint {

PROMPP_ALWAYS_INLINE void handle_current_exception(std::ostream& out, const std::source_location location = std::source_location::current()) {
  out << location.function_name() << ": ";

  try {
    std::rethrow_exception(std::current_exception());
  } catch (const BareBones::Exception& e) {
    out << e.message() << '\n' << e.stacktrace();
  } catch (const std::exception& e) {
    out << "caught a std::exception, what: " << e.what();
  } catch (...) {
    out << "caught an unknown exception";
  }
}

}  // namespace entrypoint
