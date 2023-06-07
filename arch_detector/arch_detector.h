/**
 * \file arch_detector.h
 * Contains simple architecture instruction set features detector.
 * Current supported architectures: x86-64, aarch64.
 * All detected features are defined in \ref instruction_set enum.
 */
#pragma once
#include <stdint.h>

namespace arch_detector {

/*!
  \brief Minimal set of required features from architecture instruction sets.
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

/*!
  \defgroup arch_detector.helper_macros Helper macros for compile time arch
                                        family detection.
*/

#if defined(__x86_64__) || defined(_M_AMD64)  // x64
#define ARCH_DETECTOR_BUILD_FOR_X86_64 1
#endif

#if defined(__aarch64__) || defined(_M_ARM64)  // aarch64
#define ARCH_DETECTOR_BUILD_FOR_ARM64 1
#endif  //__arm__

}  // namespace arch_detector
