#pragma once

#include <memory>
#include <vector>

#include "test.h"

class TestsDatabase {
  std::vector<std::unique_ptr<Test>> tests_;
  std::unordered_map<std::string, uint64_t> mapping_between_test_name_and_test_number;

 public:
  TestsDatabase() = default;
  TestsDatabase(const TestsDatabase&) = delete;
  TestsDatabase& operator=(const TestsDatabase&) = delete;
  TestsDatabase(TestsDatabase&&) = delete;
  TestsDatabase& operator=(TestsDatabase&&) = delete;
  ~TestsDatabase() = default;

  void add(std::unique_ptr<Test>&& test);

  std::string query(const Config& config) const;
  void run(const Config& config) const;
};
