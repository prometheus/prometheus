#pragma once

#include "performance_tests/test_with_input_only.h"

namespace performance_tests::series_index {

struct GenerateXcdatSeriesIndex : TestWithInputOnly {
  std::string input_file_base_name() const final { return "dummy_wal"; }
  bool has_output() const final { return false; }
  std::string output_file_base_name() const final { return ""; }
  void execute(const Config& config, Metrics& metrics) const final;
};

}  // namespace performance_tests::series_index
