#pragma once

#include <string>

#include "config.h"
#include "metrics.h"

struct Test {
  Test() = default;
  Test(const Test&) = delete;
  Test& operator=(const Test&) = delete;
  Test(Test&&) = delete;
  Test& operator=(Test&&) = delete;

  virtual void run(const Config&, Metrics& metrics) const = 0;
  std::string name() const;
  std::string test_data_file_name_suffix(const Config&) const;
  std::string input_file_name(const Config&) const;
  std::string output_file_name(const Config&) const;

  virtual ~Test() = default;

 protected:
  virtual std::string input_file_base_name() const = 0;
  virtual bool has_output() const = 0;
  virtual std::string output_file_base_name() const = 0;
};
