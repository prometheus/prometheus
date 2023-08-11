/// @file exception.h
/// Use this exception for wrapping any third-party libraries exceptions.
/// If you add any new object, please, generate new exception code via
/// ./scripts/err_code_gen <filename> <line>, [<git_commit_hash>]
/// from root folder.
/// Any exception throw point must contains the unique code
/// as hexadecimal digit (without delimiters, for ease of grep'ping).
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
