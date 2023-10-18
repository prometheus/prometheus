#pragma once

#include <string>
#include <unordered_map>

class Config {
  static inline const std::string NO_VALUE = "";
  std::unordered_map<std::string, std::string> params_;

 public:
  Config() = default;
  Config(const Config&) = delete;
  Config& operator=(const Config&) = delete;
  Config(Config&&) = delete;
  Config& operator=(Config&&) = delete;
  ~Config() = default;

  void parameter(const std::string& name);
  void load(char** args, int n);
  const std::string& get_value_of(const std::string& param) const;
};
