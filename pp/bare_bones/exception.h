/// @file exception.h
/// Use this exception to wrap third-party library exceptions.
/// When adding a new object, generate a new exception code using:
/// ./scripts/err_code_gen <filename> <line>
/// Execute this from the root folder.
/// Every exception thrown must have a unique hexadecimal code
/// (without delimiters) for easy grep searches.
/// Every user message must be less than 255 characters (without '\0'),
/// the longer messages would be clamped.
#pragma once

#include <sys/types.h>

#include <cstdint>
#include <exception>
#include <sstream>
#include <stacktrace>
#include <string_view>

namespace BareBones {

class Exception final : public std::exception {
 public:
  using Code = uint64_t;

  Exception(Code exc_code, const char* message, ...) __attribute__((format(printf, 3, 4)));

  [[nodiscard]] std::string_view message() const noexcept { return msg_; }
  [[nodiscard]] const std::stacktrace& stacktrace() const noexcept { return stacktrace_; }
  [[nodiscard]] const char* what() const noexcept override { return msg_.data(); }
  [[nodiscard]] Code code() const noexcept { return code_; }

 private:
  std::stacktrace stacktrace_;
  std::string_view msg_;
  Code code_;
};

}  // namespace BareBones

// C API bindings
#ifdef __cplusplus
extern "C" {
#endif  //__cplusplus

// Core Debug API

/// @brief Use it for enabling coredumps on any @ref BareBones::Exception.
/// @param enable Enables if != 0, disables otherwise.
void prompp_enable_coredumps_on_exception(int enable);

/// @brief Use it for customizing the handling of fork() in coredump logic. It may be used
///        to implement e.g., waiting logic for parsing coredumps, or extended error
///        logging/handling, etc.
void prompp_barebones_exception_set_on_fork_handler(void* state, void (*handler)(void* state, pid_t pid));

#ifdef __cplusplus
}
#endif  //__cplusplus
