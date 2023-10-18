#pragma once

#include "test_with_input_only.h"

struct load_protobuf_non_naned_wal_and_process_it_with_stale_nans : TestWithInputOnly {
  std::string input_file_base_name() const final { return "dummy_wal_for_stale_nan_test"; }
  bool has_output() const final { return false; }
  std::string output_file_base_name() const final { return ""; }
  void execute(const Config& config, Metrics& metrics) const final;
};
