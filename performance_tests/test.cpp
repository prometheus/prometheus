#include "test.h"

#include <cxxabi.h>

#include <stdexcept>

std::string Test::name() const {
  return abi::__cxa_demangle(typeid(*this).name(), nullptr, nullptr, nullptr);
}

std::string Test::input_file_name(const Config& config) const {
  return input_file_base_name() + "." + test_data_file_name_suffix(config);
}

std::string Test::output_file_name(const Config& config) const {
  if (has_output()) {
    return output_file_base_name() + "." + test_data_file_name_suffix(config);
  } else {
    return "";
  }
}

std::string Test::test_data_file_name_suffix(const Config& config) const {
  auto sort_type = config.get_value_of("input_data_ordering");
  if (sort_type == "LS_TS") {
    return "ls_ts.bin.lz4";
  } else if (sort_type == "TS_LS") {
    return "ts_ls.bin.lz4";
  } else if (sort_type == "LS") {  // NOLINT(bugprone-branch-clone): this is intentional as default order is "R"andom
    return "ls_Rts.bin.lz4";
  } else if (sort_type == "LS_RTS") {
    return "ls_Rts.bin.lz4";
  } else if (sort_type == "TS") {  // NOLINT(bugprone-branch-clone): this is intentional as default order is "R"andom
    return "ts_Rls.bin.lz4";
  } else if (sort_type == "TS_RLS") {  // NOLINT(bugprone-branch-clone): this is intentional as default order is "R"andom
    return "ts_Rls.bin.lz4";
  } else if (sort_type == "R") {
    return "R.bin.lz4";
  } else {
    throw std::runtime_error("unknown input data ordering '" + sort_type + "'");
  }
}
