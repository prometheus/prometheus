/// @file exception.h
/// Use this exception to wrap third-party library exceptions.
/// When adding a new object, generate a new exception code using:
/// ./scripts/err_code_gen <filename> <line>
/// Execute this from the root folder.
/// Every exception thrown must have a unique hexadecimal code 
/// (without delimiters) for easy grep searches.
#pragma once

#include <cstdint>
#include <exception>
#include <sstream>
#include <string>

namespace BareBones {


class Exception : public std::exception {
public:
  using Code = uint64_t;
private:
  std::string msg_;
  Code code_;

 public:
  Exception(Code exc_code, std::string_view message) : code_(exc_code) {
    std::stringstream ss;
    ss << "Exception " << std::hex << exc_code << ": " << message;
    msg_ = std::move(ss.str());
  }

  const char* what() const noexcept override { return msg_.c_str(); }
  Code code() const noexcept { return code_; }
};

}  // namespace BareBones
