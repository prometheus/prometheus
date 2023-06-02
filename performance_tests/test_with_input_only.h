#pragma once

#include "test.h"

struct TestWithInputOnly : Test {
  void run(const Config&, Metrics& metrics) const final;

 protected:
  std::string input_file_full_name(const Config& config) const;
  virtual void execute(const Config& config, Metrics& metrics) const = 0;
};
