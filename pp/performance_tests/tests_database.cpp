#include "tests_database.h"

#include <iostream>

#include "log.h"
#include "metrics.h"

void TestsDatabase::add(std::unique_ptr<Test>&& test) {
  tests_.emplace_back(std::move(test));
  const auto& registered_test = tests_.back();
  mapping_between_test_name_and_test_number.insert({registered_test->name(), tests_.size() - 1});
}

std::string TestsDatabase::query(const Config& config) const {
  const std::string& query_type = config.get_value_of("about");
  if (query_type.empty()) {
    throw std::runtime_error("query type is not defined");
  }

  if (query_type == "number_of_tests") {
    return std::to_string(tests_.size());
  } else if (query_type == "test_name") {
    if (!config.get_value_of("test_number").empty()) {
      uint64_t test_number = std::stoull(config.get_value_of("test_number"));
      if (test_number < 1 || test_number > tests_.size()) {
        throw std::runtime_error("test number should be from 1 to " + std::to_string(tests_.size()));
      }
      return tests_[test_number - 1]->name();
    } else if (!config.get_value_of("output_file_name").empty()) {
      for (const auto& test : tests_) {
        if (test->output_file_name(config) == config.get_value_of("output_file_name")) {
          return test->name();
        }
      }
      return "";
    } else {
      throw std::runtime_error("no one criteria to get test name is not defined");
    }
  } else if (query_type == "input_file_name") {
    const std::string& target = config.get_value_of("test");
    if (target.empty()) {
      throw std::runtime_error("test name or number is not defined");
    }

    const std::string& input_data_ordering = config.get_value_of("input_data_ordering");
    if (input_data_ordering.empty()) {
      throw std::runtime_error("input data sort type is not defined");
    }

    if (std::isdigit(target[0])) {
      uint64_t test_number = std::stoull(target);
      if (test_number < 1 || test_number > tests_.size()) {
        throw std::runtime_error("test number should be from 1 to " + std::to_string(tests_.size()));
      }

      return tests_[test_number - 1]->input_file_name(config);
    } else {
      if (auto it = mapping_between_test_name_and_test_number.find(target); it != mapping_between_test_name_and_test_number.end()) {
        return tests_[it->second]->input_file_name(config);
      } else {
        throw std::runtime_error("unknown name of test - '" + target + "'");
      }
    }
  } else {
    throw std::runtime_error("unknown type of queried info about tests");
  }
}

void TestsDatabase::run(const Config& config) const {
  const std::string& target = config.get_value_of("target");
  const bool should_be_quiet = config.get_value_of("quiet") == "true";
  if (target.empty()) {
    throw std::runtime_error("target to run is not defined");
  }

  const std::string& path_to_test_data = config.get_value_of("path_to_test_data");
  if (path_to_test_data.empty()) {
    throw std::runtime_error("path to test data is not defined");
  }

  const std::string& input_data_ordering = config.get_value_of("input_data_ordering");
  if (input_data_ordering.empty()) {
    throw std::runtime_error("input data ordering is not defined");
  }

  const std::string& branch = config.get_value_of("branch");
  if (!should_be_quiet && branch.empty()) {
    throw std::runtime_error("branch is not defined");
  }

  if (!should_be_quiet && config.get_value_of("metrics_file_full_name").empty()) {
    throw std::runtime_error("metrics file name is not defined");
  }

  Metrics metrics(config);
  if (config.get_value_of("commit_hash").empty()) {
    Metric::define_global_label(Metric::Label{"samples_ordering", input_data_ordering});
    Metric::define_global_label(Metric::Label{"branch", branch});
  } else {
    Metric::define_global_label(Metric::Label{"samples_ordering", input_data_ordering});
    Metric::define_global_label(Metric::Label{"branch", branch});
    Metric::define_global_label(Metric::Label{"commit_hash", config.get_value_of("commit_hash")});
  }

  Logger::init(config);

  if (std::isdigit(target[0])) {
    uint64_t test_number = std::stoull(target);
    if (test_number < 1 || test_number > tests_.size()) {
      throw std::runtime_error("test number should be from 1 to " + std::to_string(tests_.size()));
    }

    tests_[test_number - 1]->run(config, metrics);
  } else {
    if (auto it = mapping_between_test_name_and_test_number.find(target); it != mapping_between_test_name_and_test_number.end()) {
      tests_[it->second]->run(config, metrics);
    } else {
      throw std::runtime_error("unknown name of test - '" + target + "'");
    }
  }
}
