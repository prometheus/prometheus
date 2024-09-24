#pragma once

#define PROMPP_ATTRIBUTE_INLINE __attribute__((always_inline))
#define PROMPP_ATTRIBUTE_NOINLINE __attribute__((noinline))
#define PROMPP_ALWAYS_INLINE inline PROMPP_ATTRIBUTE_INLINE
#define PROMPP_LAMBDA_INLINE PROMPP_ATTRIBUTE_INLINE

#define PROMPP_ATTRIBUTE_PACKED __attribute__((__packed__))

#define PROMPP_STRINGIFY(a) #a

#ifdef __clang__
#define DIAGNOSTIC_CLASS_MEMACCESS "-Wdynamic-class-memaccess"
#define PRAGMA_DIAGNOSTIC(value) _Pragma(PROMPP_STRINGIFY(clang diagnostic value))
#elif __GNUC__
#define DIAGNOSTIC_CLASS_MEMACCESS "-Wclass-memaccess"
#define PRAGMA_DIAGNOSTIC(value) _Pragma(PROMPP_STRINGIFY(GCC diagnostic value))
#else
#error #pragma diagnostic is not supported
#endif
