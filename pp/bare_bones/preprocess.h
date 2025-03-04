#pragma once

#define PROMPP_ATTRIBUTE_INLINE __attribute__((always_inline))
#define PROMPP_ATTRIBUTE_NOINLINE __attribute__((noinline))

#ifdef NDEBUG
#define PROMPP_ALWAYS_INLINE inline PROMPP_ATTRIBUTE_INLINE
#define PROMPP_LAMBDA_INLINE PROMPP_ATTRIBUTE_INLINE
#else
#define PROMPP_ALWAYS_INLINE inline PROMPP_ATTRIBUTE_NOINLINE
#define PROMPP_LAMBDA_INLINE PROMPP_ATTRIBUTE_NOINLINE
#endif

#define PROMPP_ATTRIBUTE_PACKED __attribute__((__packed__))

#define PROMPP_STRINGIFY(a) #a

#ifdef __clang__
#define DIAGNOSTIC_CLASS_MEMACCESS "-Wdynamic-class-memaccess"
#define DIAGNOSTIC_MAYBE_UNINITIALIZED "-Wuninitialized"
#define PRAGMA_DIAGNOSTIC(value) _Pragma(PROMPP_STRINGIFY(clang diagnostic value))
#elif __GNUC__
#define DIAGNOSTIC_CLASS_MEMACCESS "-Wclass-memaccess"
#define DIAGNOSTIC_MAYBE_UNINITIALIZED "-Wmaybe-uninitialized"
#define PRAGMA_DIAGNOSTIC(value) _Pragma(PROMPP_STRINGIFY(GCC diagnostic value))
#else
#error #pragma diagnostic is not supported
#endif


#if __has_include(<gtest/gtest.h>)
constexpr auto kIsUnitTestBuild = true;
#else
constexpr auto kIsUnitTestBuild = false;
#endif

#ifdef __SANITIZE_ADDRESS__
constexpr auto kIsBuildWithAsan = true;
#else
constexpr auto kIsBuildWithAsan = false;
#endif
