package(default_visibility = ["//visibility:public"])
load("//:bazel/rules/cc_static_library.bzl", "cc_static_library")

config_setting(
    name = "aarch64_build",
    values = {
        "cpu": "aarch64"
    },
)

config_setting(
    name = "x86_build",
    values = {
        "cpu": "x86",
    },
)

cc_library(
    name = "arch_detector",
    hdrs = glob(["arch_detector/*.h"]),
    srcs = glob(["arch_detector/*.cpp"]),
)

cc_test(
    name = "arch_detector_test",
    srcs = glob(["arch_detector/tests/*_tests.cpp"]),
    deps = [
        ":arch_detector",
        "@gtest//:gtest_main"
    ],
)

cc_library(
    name = "bare_bones_headers",
    hdrs = glob(["bare_bones/*.h"]),
    deps = [
        "//third_party",
        "@parallel_hashmap",
        "@scope_exit",
        "@backward_cpp//:backward_cpp_header_only", # stacktrace lib
    ],
)

cc_library(
    name  = "bare_bones_exceptions",
    srcs = ["bare_bones/exception.cpp"],
    deps = [
        ":bare_bones_headers",
    ],
)

cc_library(
    name = "bare_bones",
    deps = [
        ":bare_bones_headers",
        ":bare_bones_exceptions",
    ],
)

cc_test(
    name = "bare_bones_test",
    srcs = glob(["bare_bones/tests/*_tests.cpp"]),
    malloc = "@jemalloc",
    deps = [
        ":bare_bones",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "primitives",
    hdrs = glob(["primitives/*.h"]),
    deps = [
        ":bare_bones",
    ],
)

cc_test(
    name = "primitives_test",
    srcs = glob(["primitives/tests/*_tests.cpp"]),
    malloc = "@jemalloc",
    deps = [
        ":primitives",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "prometheus",
    hdrs = glob(["prometheus/*.h"]),
    deps = [
        ":bare_bones",
        ":primitives",
    ],
)

cc_test(
    name = "prometheus_test",
    srcs = glob(["prometheus/tests/*_tests.cpp"]),
    malloc = "@jemalloc",
    deps = [
        ":prometheus",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "wal",
    hdrs = glob(["wal/*.h"]),
    deps = [
        ":bare_bones",
        ":primitives",
        ":prometheus",
    ],
)

cc_test(
    name = "wal_test",
    srcs = glob(["wal/tests/*_tests.cpp"]),
    malloc = "@jemalloc",
    deps = [
        ":wal",
        "@gtest//:gtest_main",
    ],
)

# Define C Bindings targets
# We are exporting multiple flavours of C Bindings
# 1. Generic x86
# 2. SSE42 (Nehalem)
# 3. Haswell+ (first CPU with BMI1 + AVX2)
# 4. Generic ARM
# 5. ARM with CRC32
cc_library(
    name = "x86_generic_wal_c_api",
    hdrs = glob(["wal/wal_c_api/*.h"]),
    srcs = glob(["wal/wal_c_api/*.cpp"]),
    local_defines = ["OKDB_WAL_FUNCTION_NAME_PREFIX=x86_generic_"],
    deps = [
        "wal",
        "arch_detector",
    ],
    target_compatible_with = [
        "@platforms//cpu:x86_64"
    ],
)

cc_library(
    name = "x86_nehalem_wal_c_api",
    hdrs = glob(["wal/wal_c_api/*.h"]),
    srcs = glob(["wal/wal_c_api/*.cpp"]),
    local_defines = ["OKDB_WAL_FUNCTION_NAME_PREFIX=x86_nehalem_"],
    deps = [
        "wal",
        "arch_detector",
    ],
    copts = ["-march=nehalem"],
    target_compatible_with = [
        "@platforms//cpu:x86_64"
    ],
)

cc_library(
    name = "x86_haswell_wal_c_api",
    hdrs = glob(["wal/wal_c_api/*.h"]),
    srcs = glob(["wal/wal_c_api/*.cpp"]),
    local_defines = ["OKDB_WAL_FUNCTION_NAME_PREFIX=x86_haswell_"],
    deps = [
        "wal",
        "arch_detector",
    ],
    copts = ["-march=haswell"],
    target_compatible_with = [
        "@platforms//cpu:x86_64"
    ],
)

cc_library(
    name = "x86_wal_c_api",
    deps = [
        ":x86_generic_wal_c_api",
        ":x86_haswell_wal_c_api",
        ":x86_nehalem_wal_c_api",
    ],
)

cc_static_library(
    name = "x86_wal_c_api_static",
    deps = [
        ":x86_generic_wal_c_api",
        ":x86_haswell_wal_c_api",
        ":x86_nehalem_wal_c_api",
        ":wal_c_api", # for multiarch objs
    ],
)

cc_library(
    name = "aarch64_generic_wal_c_api",
    hdrs = glob(["wal/wal_c_api/*.h"]),
    srcs = glob(["wal/wal_c_api/*.cpp"]),
    local_defines = ["OKDB_WAL_FUNCTION_NAME_PREFIX=aarch64_generic_"],
    deps = [
        "wal",
        "arch_detector",
    ],
    copts = ["-march=armv8-a"],
    target_compatible_with = [
        "@platforms//cpu:arm64"
    ],
)

cc_library(
    name = "aarch64_crc_wal_c_api",
    hdrs = glob(["wal/wal_c_api/*.h"]),
    srcs = glob(["wal/wal_c_api/*.cpp"]),
    local_defines = ["OKDB_WAL_FUNCTION_NAME_PREFIX=aarch64_crc_"],
    deps = [
        "wal",
        "arch_detector",
    ],
    copts = ["-march=armv8-a+crc"],
    target_compatible_with = [
        "@platforms//cpu:arm64"
    ],
)

cc_library(
    name = "aarch64_wal_c_api",
    deps = [
        ":aarch64_generic_wal_c_api",
        ":aarch64_crc_wal_c_api",
    ],
)

cc_static_library(
    name = "aarch64_wal_c_api_static",
    deps = [
        ":aarch64_generic_wal_c_api",
        ":aarch64_crc_wal_c_api",
        ":wal_c_api", # for multiarch objs
    ],
)

cc_library(
    name = "wal_c_api",
    hdrs = ["wal/wal_c_api.h", "wal/wal_c_types_api.h"],
    srcs = ["wal/wal_c_api.cpp", "wal/wal_c_types.cpp", "wal/wal_c_go_uni_api.cpp"],

    deps = [":arch_detector"] +
    select({
        ":aarch64_build": ["aarch64_wal_c_api"],
        ":x86_build": ["x86_wal_c_api"],
        "//conditions:default" : ["x86_wal_c_api"],
    }),
)

cc_test(
    name = "wal_c_api_test",
    srcs = glob(["wal/wal_c_api/tests/*_tests.cpp"]),
    deps = [
        ":wal_c_api",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "delivery",
    hdrs = glob(["go/delivery/libcpp/*.h"]),
    srcs = glob(["go/delivery/libcpp/*.cpp"]),
    deps = [
        ":wal_c_api",
    ],
)

cc_test(
    name = "delivery_test",
    srcs = glob(["go/delivery/libcpp/tests/*_tests.cpp"]),
    deps = [
        ":delivery",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "performance_tests_headers",
    hdrs = glob(["performance_tests/*.h"]),
    deps = [
        ":wal",
        ":prometheus",
        "@lz4stream//:lz4stream",
        "//third_party:third_party",
    ],
)

cc_binary(
    name = "performance_tests",
    srcs = glob(["performance_tests/*.cpp"]),
    malloc = "@jemalloc",
    deps = [
        ":performance_tests_headers"
    ],
)

cc_library(
    name = "integration_tests_headers",
    hdrs = glob(["integration_tests/*.h"]),
    deps = [
        ":primitives",
        "@lz4stream//:lz4stream",
        "@gtest//:gtest_main",
    ],
)

cc_binary(
    name = "integration_tests",
    srcs = glob(["integration_tests/*.cpp"]),
    malloc = "@jemalloc",
    deps = [
        ":integration_tests_headers"
    ]
)
