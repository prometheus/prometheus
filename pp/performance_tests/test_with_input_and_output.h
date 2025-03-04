#pragma once

#include "metrics.h"
#include "test.h"

struct TestWithInputAndOutput : Test {
  void run(const Config&, Metrics& metrics) const final;

 protected:
  std::string input_file_full_name(const Config& config) const;
  std::string output_file_full_name(const Config& config) const;
  virtual void execute(const Config& config, Metrics& metrics) const = 0;
  std::string output_file_base_name() const override = 0;
};
