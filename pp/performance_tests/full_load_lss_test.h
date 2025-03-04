#pragma once

#include "test_with_input_only.h"

struct full_load_lss : public TestWithInputOnly {
  std::string input_file_base_name() const final { return "lss_full"; }
  bool has_output() const final { return false; }
  std::string output_file_base_name() const final { return ""; }
  void execute(const Config& config, Metrics& metrics) const final;
};
