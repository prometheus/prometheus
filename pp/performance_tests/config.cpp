#include "config.h"

#include <stdexcept>

void Config::parameter(const std::string& name) {
  params_.insert({name, ""});
}

void Config::load(char** args, int n) {
  for (int i = 0; i < n; i += 2) {
    // NOLINTNEXTLINE(cppcoreguidelines-pro-bounds-pointer-arithmetic): these pointers are from argc/argv, so silence it.
    if (auto it = params_.find(args[i]); it != params_.end()) {
      it->second = args[i + 1];  // NOLINT(cppcoreguidelines-pro-bounds-pointer-arithmetic)
    } else {
      // NOLINTNEXTLINE(cppcoreguidelines-pro-bounds-pointer-arithmetic)
      throw std::runtime_error("unknown parameter - '" + std::string(args[i]) + "'");
    }
  }
}

const std::string& Config::get_value_of(const std::string& param) const {
  if (auto it = params_.find(param); it != params_.end()) {
    return it->second;
  } else {
    return NO_VALUE;
  }
}
