#pragma once

#include "test_with_input_only.h"

struct load_protobuf_wal_and_save_gorilla_to_sharded_wal : TestWithInputOnly {
  std::string input_file_base_name() const final { return "protobuf_wal"; }
  bool has_output() const final { return false; }
  std::string output_file_base_name() const final { return ""; }
  void execute(const Config& config, Metrics& metrics) const final;

 private:
  std::vector<size_t> numbers_of_shards_ = {1, 2, 4, 8, 16, 32, 64, 128, 256};
};
