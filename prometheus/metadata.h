#pragma once

#include <cstdint>

namespace PromPP::Prometheus {

enum class MetadataType : uint8_t {
  kHelp = 0,
  kType,
};

}  // namespace PromPP::Prometheus