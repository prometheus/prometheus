#include "configuration.h"

#include <cstdlib>
#include <stdexcept>

namespace Configuration {

std::string get_path_to_test_data() {
  if (auto val = std::getenv("INTEGRATION_TESTS_PATH_TO_TEST_DATA"); val != nullptr) {
    return val;
  } else {
    throw std::runtime_error("Environment variable INTEGRATION_TESTS_PATH_TO_TEST_DATA is not set");
  }
}

std::string get_input_data_ordering() {
  if (auto val = std::getenv("INTEGRATION_TESTS_INPUT_DATA_ORDERING"); val != nullptr) {
    return val;
  } else {
    throw std::runtime_error("Environment variable INTEGRATION_TESTS_INPUT_DATA_ORDERING is not set");
  }
}

}  // namespace Configuration
