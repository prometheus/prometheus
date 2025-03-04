#pragma once

#include "test_with_input_only.h"

struct load_lss_from_wal : TestWithInputOnly {
  std::string input_file_base_name() const final { return "lss_wal"; }
  bool has_output() const final { return false; }
  std::string output_file_base_name() const final { return ""; }
  void execute(const Config& config, Metrics& metrics) const;
};
