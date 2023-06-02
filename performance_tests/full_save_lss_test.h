#pragma once

#include "test_with_input_and_output.h"

struct full_save_lss : TestWithInputAndOutput {
  std::string input_file_base_name() const final { return "lss_wal"; }
  bool has_output() const final { return true; }
  std::string output_file_base_name() const final { return "lss_full"; }
  void execute(const Config& config, Metrics& metrics) const final;
};
