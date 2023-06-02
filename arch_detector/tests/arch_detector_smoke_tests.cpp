#include <gtest/gtest.h>

#include "arch_detector/arch_detector.h"

#include <filesystem>
#include <fstream>
#include <string>

// check that detector is actually detects required features...
// The reference values we will get from /proc/cpuinfo (or return 'generic')
namespace {

[[maybe_unused]] std::string read_cpu_features_from_procinfo() {
  std::ifstream cpuinfo("/proc/cpuinfo");
  if (!cpuinfo) {
    return "";
  }
  std::string result;
  while (std::getline(cpuinfo, result)) {
    if (result.starts_with("flags")) {
      break;
    }
  }
  auto colon_position = result.find(':');
  if (colon_position == decltype(result)::npos) {
    return "";
  }
  if (colon_position + 1 >= result.size()) {
    return "";
  }

  return result.substr(colon_position + 1);
}

}  // namespace

TEST(ArchDetector, SmokeTest) {
  auto arch_instr_sets = arch_detector::detect_supported_architectures();
  EXPECT_EQ(arch_instr_sets & arch_detector::SSE42, arch_detector::SSE42);
}
