#pragma once

#include <stdexcept>
#include <string>

inline std::string test_file_name_suffix(const std::string ordering_type) {
  if (ordering_type == "TS_LS") {
    return "ts_ls.bin.lz4";
  } else if (ordering_type == "LS_TS") {
    return "ls_ts.bin.lz4";
  } else if (ordering_type == "LS") {
    return "ls_Rts.bin.lz4";
  } else if (ordering_type == "TS") {
    return "ts_Rls.bin.lz4";
  } else if (ordering_type == "R") {
    return "R.bin.lz4";
  } else {
    throw std::runtime_error("Unknown ordering type");
  }
}