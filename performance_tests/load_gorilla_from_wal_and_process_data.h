#pragma once

#include <chrono>

#include "test_with_input_only.h"
#include "wal/wal.h"

struct load_gorilla_from_wal_and_process_data : public TestWithInputOnly {
  std::string input_file_base_name() const final { return "wal"; }
  bool has_output() const final { return false; }
  std::string output_file_base_name() const final { return ""; }
  void execute(const Config& config, Metrics& metrics) const final;

 private:
  virtual std::chrono::nanoseconds process_data(PromPP::WAL::Reader& wal) const = 0;
  virtual void write_metrics(Metrics&) const = 0;
};
