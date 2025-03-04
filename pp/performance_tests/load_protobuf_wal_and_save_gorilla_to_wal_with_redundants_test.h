#pragma once

#include "test_with_input_and_output.h"

struct load_protobuf_wal_and_save_gorilla_to_wal_with_redundants : TestWithInputAndOutput {
  std::string input_file_base_name() const final { return "protobuf_wal"; }
  bool has_output() const final { return true; }
  std::string output_file_base_name() const final { return "wal"; }
  void execute(const Config& config, Metrics& metrics) const final;
};
