#pragma once

#include "test_with_input_and_output.h"

struct save_gorilla_to_wal : TestWithInputAndOutput {
  std::string input_file_base_name() const final { return "dummy_wal"; }
  bool has_output() const final { return true; }
  std::string output_file_base_name() const final { return "wal"; }
  void execute(const Config& config, Metrics& metrics) const final;
};
