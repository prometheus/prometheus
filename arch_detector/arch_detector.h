#pragma once
#include <stdint.h>

namespace arch_detector {

/*!
  \note Some flags has been cleaned for the sake of easiness of supporting
        only required architectures' flavours.
 */
enum instruction_set {
  DEFAULT = 0x0,
  NEON = 0x1,
  AVX2 = 0x4,
  SSE42 = 0x8,
  CRC32 = 0x10,  // also set if SSE42 avail on x86 (it has crc32)
  BMI1 = 0x20,
};

uint32_t detect_supported_architectures();

}  // namespace arch_detector
