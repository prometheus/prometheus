load("//:bazel/rules/cc_static_library.bzl", "cc_static_library")

package(default_visibility = ["//visibility:public"])

filegroup(
    name = "clang_tidy_config_default",
    srcs = [
        ".clang-tidy",
    ],
)

label_flag(
    name = "clang_tidy_config",
    build_setting_default = ":clang_tidy_config_default",
    visibility = ["//visibility:public"],
)

cc_library(
    name = "bare_bones_headers",
    hdrs = glob(["bare_bones/*.h"]),
    deps = [
        "//third_party",
        "@lz4",
        "@parallel_hashmap",
        "@scope_exit",
        "@xxHash",
        "@roaring",
    ],
)

cc_library(
    name = "bare_bones_exceptions",
    srcs = ["bare_bones/exception.cpp"],
    deps = [
        ":bare_bones_headers",
    ],
)

cc_library(
    name = "bare_bones",
    deps = [
        ":bare_bones_exceptions",
        ":bare_bones_headers",
    ],
)

cc_test(
    name = "bare_bones_test",
    srcs = glob(["bare_bones/tests/*_tests.cpp"]),
    deps = [
        ":bare_bones",
        "@gtest//:gtest_main",
    ],
)

cc_binary(
    name = "bare_bones_coredump_test",
    srcs = [
        "bare_bones/tests/coredump_test_separate.cpp",
    ],
    deps = [
        ":bare_bones",
    ],
)

cc_library(
    name = "primitives",
    hdrs = glob(["primitives/**/*.h"]),
    deps = [
        ":bare_bones",
    ],
)

cc_test(
    name = "primitives_test",
    srcs = glob(["primitives/tests/*_tests.cpp"]),
    deps = [
        ":primitives",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "prometheus",
    srcs = glob(
        include = ["prometheus/**/*.cpp"],
        exclude = ["prometheus/**/*_tests.cpp"],
    ),
    hdrs = glob([
        "prometheus/**/*.h",
        "prometheus/**/*.hpp",
    ]),
    deps = [
        ":bare_bones",
        ":primitives",
        "@com_google_absl//absl/crc:crc32c",
        "@quasis_crypto",
        "@re2",
        "@simdutf",
    ],
)

cc_test(
    name = "prometheus_test",
    srcs = glob(["prometheus/**/*_tests.cpp"]),
    deps = [
        ":prometheus",
        "@gtest//:gtest_main",
        "@roaring",
    ],
)

cc_library(
    name = "wal",
    hdrs = glob(["wal/**/*.h"]),
    deps = [
        ":bare_bones",
        ":primitives",
        ":prometheus",
        "@fastfloat",
        "@roaring",
        "@simdutf",
        "@snappy",
    ],
)

cc_test(
    name = "wal_test",
    srcs = glob(["wal/**/*_tests.cpp"]),
    deps = [
        ":wal",
        "@gtest//:gtest_main",
    ],
)

cc_binary(
    name = "wal_benchmark",
    srcs = glob(["wal/**/*_benchmark.cpp"]),
    deps = [
        ":wal",
        "@google_benchmark//:benchmark_main",
    ],
)

cc_library(
    name = "head",
    hdrs = glob(["head/**/*.h"]),
    linkstatic = True,
    deps = [
        ":series_data",
        ":series_index",
    ],
)

cc_test(
    name = "head_test",
    srcs = glob(["head/**/*_tests.cpp"]),
    deps = [
        ":head",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "entrypoint",
    srcs = glob(
        include = ["entrypoint/**/*.cpp"],
        exclude = [
            "entrypoint/init/**",
            "entrypoint/**/*_tests.cpp",
        ],
    ),
    hdrs = glob([
        "entrypoint/**/*.h",
        "entrypoint/**/*.hpp",
    ]),
    linkstatic = True,
    deps = [
        ":bare_bones",
        ":head",
        ":primitives",
        ":wal",
    ] + select({
        "//bazel/toolchain:with_asan": [],
        "//conditions:default": ["@jemalloc"],
    }),
)

cc_static_library(
    name = "entrypoint_aio",
    deps = [
        ":entrypoint",
    ],
)

cc_library(
    name = "entrypoint_init",
    srcs = glob(["entrypoint/init/*.cpp"]),
    hdrs = glob([
        "entrypoint/init/*.h",
        "entrypoint/init/*.hpp",
    ]),
    linkstatic = True,
)

cc_static_library(
    name = "entrypoint_init_aio",
    deps = [
        ":entrypoint_init",
    ],
)

cc_library(
    name = "performance_tests_headers",
    hdrs = glob(["performance_tests/**/*.h"]),
    deps = [
        ":primitives",
        ":prometheus",
        ":series_data",
        ":series_index",
        ":wal",
        "//third_party",
    ],
)

cc_binary(
    name = "performance_tests_bin",
    srcs = glob(["performance_tests/**/*.cpp"]),
    malloc = "@jemalloc",
    deps = [
        ":performance_tests_headers",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "integration_tests_headers",
    hdrs = glob(["integration_tests/*.h"]),
    deps = [
        ":primitives",
        "@gtest//:gtest_main",
    ],
)

cc_binary(
    name = "integration_tests",
    srcs = glob(["integration_tests/*.cpp"]),
    malloc = "@jemalloc",
    deps = [
        ":integration_tests_headers",
    ],
)

cc_library(
    name = "ceph_sdk",
    srcs = glob(["ceph/sdk/**/*.cpp"]),
    hdrs = glob(["ceph/sdk/**/*.h"]),
    includes = ["./ceph"],
    deps = [
        ":bare_bones_headers",
        "@snappy",
    ],
)

cc_library(
    name = "cls_common",
    hdrs = glob(["ceph/cls/common/**/*.h"]),
    includes = ["./ceph/cls"],
    deps = [
        ":ceph_sdk",
        "@lru_cache",
    ],
)

cc_test(
    name = "cls_common_test",
    srcs = glob([
        "ceph/cls/common/tests/**/*.cpp",
    ]),
    deps = [
        ":cls_common",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "cls_wal_modules",
    hdrs = glob([
        "ceph/cls/cls_wal/modules/**/*.h",
        "ceph/cls/cls_wal/config/**/*.h",
        "ceph/cls/cls_wal/constants.h",
        "ceph/cls/cls_wal/tests/**/*.h",
    ]),
    includes = ["ceph/cls/cls_wal"],
    deps = [
        ":bare_bones_headers",
        ":cls_common",
        ":prometheus",
        ":series_data",
        ":series_index",
        ":wal",
    ],
)

cc_binary(
    name = "cls_wal",
    srcs = glob(["ceph/cls/cls_wal/*.cpp"]),
    linkshared = True,
    deps = [
        ":ceph_sdk",
        "@ceph",
        ":cls_wal_modules",
    ],
)

cc_test(
    name = "cls_wal_test",
    srcs = glob([
        "ceph/cls/cls_wal/tests/**/*.cpp",
    ]),
    deps = [
        ":cls_wal_modules",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "series_index",
    hdrs = glob(["series_index/**/*.h"]),
    deps = [
        ":bare_bones_headers",
        ":prometheus",
        ":wal",
        "@cedar",
        "@re2",
    ],
)

cc_test(
    name = "series_index_test",
    srcs = glob([
        "series_index/tests/**/*.cpp",
    ]),
    deps = [
        ":series_index",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "series_data",
    hdrs = glob(["series_data/**/*.h"]),
    deps = [
        ":bare_bones_headers",
        ":primitives",
    ],
)

cc_test(
    name = "series_data_test",
    srcs = glob([
        "series_data/tests/**/*.cpp",
    ]),
    deps = [
        ":series_data",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "cls_block_catalog_modules",
    hdrs = glob([
        "ceph/cls/block_catalog/modules/**/*.h",
        "ceph/cls/block_catalog/constants.h",
        "ceph/cls/block_catalog/perf_counters.h",
        "ceph/cls/block_catalog/tests/**/*.h",
    ]),
    includes = ["ceph/cls/block_catalog"],
    deps = [
        ":bare_bones_headers",
        ":cls_common",
        ":prometheus",
    ],
)

cc_binary(
    name = "cls_block_catalog",
    srcs = glob([
        "ceph/cls/block_catalog/*.cpp",
    ]),
    linkshared = True,
    deps = [
        ":cls_block_catalog_modules",
        "@ceph",
    ],
)

cc_test(
    name = "cls_block_catalog_test",
    srcs = glob([
        "ceph/cls/block_catalog/tests/**/*.cpp",
    ]),
    deps = [
        ":cls_block_catalog_modules",
        "@gtest//:gtest_main",
    ],
)
