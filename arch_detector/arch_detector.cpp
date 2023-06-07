// SPDX-License-Identifier: BSD-3-Clause
// Partially inspired by github.com/simdjson/simdjson.
/*!
  \file arch_detector.cpp Contains implementation for arch flavours detection.
                          Defines the main public C API
                          \ref arch_detector_detect_supported_architectures().
*/
#include "arch_detector.h"

#include <cstdint>
#include <cstdlib>

// impl API fwd declaration.
namespace arch_detector {
uint32_t detect_supported_architectures();
}  // namespace arch_detector

/*!
  \brief The main API for detecting instruction sets' features.
  \return Bit flags with \ref arch_detector::instruction_set values.
*/
extern "C" uint32_t arch_detector_detect_supported_architectures() {
  return arch_detector::detect_supported_architectures();
}

//
// Impl...

#if ARCH_DETECTOR_BUILD_FOR_X86_64  // x64
#if __has_include(<cpuid.h>)        // intel x86/x86-64 specific #include.
#include <cpuid.h>
#else
#error "<cpuid.h> is not available for your toolchain"
#endif
#endif

namespace arch_detector {

#if ARCH_DETECTOR_BUILD_FOR_ARM64
// Detect CRC32 via getauxval(3) which uses the /proc/self/auxv (linux kernel only)
// Note that if you need 32 bit support, you must use HWCAP2_CRC32, not HWCAP_CRC32!
// This function is available since glibc 2.16 (and has a minor bug with errno until 2.19).
// See https://man7.org/linux/man-pages/man3/getauxval.3.html for details.

// If /proc/self/auxv is not available (e.g., Android forbids its usage), then we would return NEON for now
// Futher references:
// 1. zlib's author's question about arm and crc32:
//    https://stackoverflow.com/questions/53965723/how-to-detect-crc32-on-aarch64
// 2. Go runtime with example how to handle unavailable `/proc/self/auxv`:
//    https://cs.opensource.google/go/x/sys/+/refs/tags/v0.8.0:cpu/cpu_linux_arm64.go;l=59
// 3. Chromium's detection of CRC32 (using auxv):
//    https://source.chromium.org/chromium/chromium/src/+/main:third_party/zlib/arm_features.c;l=29;drc=9d8f976414a7608c3361718462253104a761c6bb
// 4. Google's implementation which handles ancient glibc libs:
//    https://github.com/google/cpu_features/blob/41e206e435b3c84a6fdd937dfe2a07e8ee73e611/src/hwcaps.c#L154
// 5. Reference code from ARM blogs:
// https://community.arm.com/arm-community-blogs/b/operating-systems-blog/posts/runtime-detection-of-cpu-features-on-an-armv8-a-cpu
//
#if __has_include(<sys/auxv.h>)
#define HAS_AUXV_H_
#include <sys/auxv.h>
#else
#error "<sys/auxv.h> is not available for your platform (It's Linux-specific and requires glibc>=2.16)"
#endif

// Linux header with hardware capabilities (ARM-specific file, from linux-headers)
#if __has_include(<asm/hwcap.h>)
#include <asm/hwcap.h>
#else
#error "<asm/hwcap.h> is not available for your platform (It's ARM Linux specific header, check linux-headers)"
#endif

// Tries to open /proc/self/auxval and determine the CRC32.
// On fail, only NEON is returned.
uint32_t detect_supported_architectures() {
  // note: then we use aarch64 with 64bits,
  // and we should use AT_HWCAP, not AT_HWCAP2!
  auto hwcaps = getauxval(AT_HWCAP);
  bool has_crc32 = (hwcaps & HWCAP_CRC32);

  return instruction_set::NEON | (has_crc32 ? instruction_set::CRC32 : 0);
}

#elif ARCH_DETECTOR_BUILD_FOR_X86_64  // ^__aarch64 /  x64 ---v

namespace {

// Can be found on Intel ISA Reference for CPUID
constexpr uint32_t CPUID_AVX2_BIT = 1 << 5;    ///< \private Bit 5 of EBX for EAX=0x7
constexpr uint32_t CPUID_BMI1_BIT = 1 << 3;    ///< \private bit 3 of EBX for EAX=0x7
constexpr uint32_t CPUID_SSE42_BIT = 1 << 20;  ///< \private bit 20 of ECX for EAX=0x1
}  // namespace

void cpuid(uint32_t* eax, uint32_t* ebx, uint32_t* ecx, uint32_t* edx) {
  uint32_t level = *eax;
  __get_cpuid(level, eax, ebx, ecx, edx);
}

uint32_t detect_supported_architectures() {
  uint32_t eax, ecx;
  uint32_t ebx = 0;
  uint32_t edx = 0;

  // out flags
  uint32_t host_isa = 0x0;

  // ECX for EAX=0x7
  eax = 0x7;
  ecx = 0x0;
  cpuid(&eax, &ebx, &ecx, &edx);
  if (ebx & CPUID_AVX2_BIT) {
    host_isa |= instruction_set::AVX2;
  }
  if (ebx & CPUID_BMI1_BIT) {
    host_isa |= instruction_set::BMI1;
  }

  // EBX for EAX=0x1
  eax = 0x1;
  cpuid(&eax, &ebx, &ecx, &edx);

  if (ecx & CPUID_SSE42_BIT) {
    host_isa |= (instruction_set::SSE42 | instruction_set::CRC32);
  }
  return host_isa;
}
#else                                 // fallback

uint32_t detect_supported_architectures() {
  return instruction_set::DEFAULT;
}

#endif  // end SIMD extension detection code

}  // namespace arch_detector
