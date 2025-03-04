#pragma once

#include "test_with_input_and_output.h"

struct write_protobuf_non_naned_wal : public TestWithInputAndOutput {
  std::string input_file_base_name() const final { return "dummy_wal"; }
  bool has_output() const final { return true; }
  std::string output_file_base_name() const final { return "dummy_wal_for_stale_nan_test"; }
  void execute(const Config& config, Metrics& metrics) const final;
};
