#include "test_with_input_only.h"

#include <filesystem>
#include <iostream>
#include <stdexcept>

#include "log.h"

std::string TestWithInputOnly::input_file_full_name(const Config& config) const {
  return config.get_value_of("path_to_test_data") + "/" + input_file_base_name() + "." + test_data_file_name_suffix(config);
}

void TestWithInputOnly::run(const Config& config, Metrics& metrics) const {
  if (!std::filesystem::exists(input_file_full_name(config))) {
    throw std::runtime_error("input file '" + input_file_full_name(config) + "' for test '" + name() + "' does not exist");
  }

  log() << "Run test " << name() << "!" << std::endl;
  log() << "\tinput data file: " << input_file_full_name(config) << std::endl;
  execute(config, metrics);
}
