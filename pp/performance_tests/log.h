#pragma once

#include <iomanip>
#include <iostream>

#include "config.h"

class Logger {
  bool should_be_quiet_;

 public:
  Logger(const Logger&) = delete;
  Logger& operator=(const Logger&) = delete;
  Logger(Logger&&) = delete;
  Logger& operator=(Logger&&) = delete;
  ~Logger() = default;

  static void init(const Config& config);

  template <typename T>
  Logger& operator<<(const T& arg) {
    if (!should_be_quiet_) {
      // NOLINTNEXTLINE(cppcoreguidelines-pro-bounds-array-to-pointer-decay): ignore array decaying to pointer.
      std::cout << arg;
    }
    return *this;
  }

  Logger& operator<<(std::ostream& (*pf)(std::ostream&)) {
    if (!should_be_quiet_) {
      std::cout << pf;
    }
    return *this;
  }

 private:
  Logger();
  friend Logger logger_instance();
  void configure(const Config& config);
};

Logger& log();
