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
        "@parallel_hashmap",
        "@scope_exit",
        # "@zlib//:zlib",
        # "@lzma//:lzma",
        # "@elf//:elf",
        # "@backward_cpp//:backward_cpp_header_only", # stacktrace lib
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
    name = "lz4chain",
    hdrs = glob(["bare_bones/lz4chain/*.h"]),
    deps = [
        ":bare_bones_headers",
        "@lz4//:lz4"
    ],
)

cc_test(
    name = "lz4chain_test",
    srcs = glob(["bare_bones/lz4chain/tests/*_tests.cpp"]),
    malloc = "@jemalloc",
    deps = [
        ":lz4chain",
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
    deps = [
        ":prometheus",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "wal",
    hdrs = glob(["wal/*.h"]),
    deps = [
        "@roaring",
        ":bare_bones",
        ":primitives",
        ":prometheus",
    ],
)

cc_test(
    name = "wal_test",
    srcs = glob(["wal/tests/*_tests.cpp"]),
    deps = [
        ":wal",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "entrypoint",
    hdrs = glob([
        "entrypoint/*.h",
        "entrypoint/*.hpp",
    ]),
    srcs = glob(["entrypoint/*.cpp"]),
    deps = [
        ":bare_bones",
        ":primitives",
        ":wal",
    ] + select({
        "//bazel/toolchain:with_asan": [],
        "//conditions:default": ["@jemalloc"],
    }),
    linkstatic = True,
)

cc_static_library(
    name = "entrypoint_aio",
    deps = [
        ":entrypoint",
    ],
)

cc_library(
    name = "entrypoint_init",
    hdrs = glob([
        "entrypoint/init/*.h",
        "entrypoint/init/*.hpp",
    ]),
    srcs = glob(["entrypoint/init/*.cpp"]),
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
    hdrs = glob(["performance_tests/*.h"]),
    deps = [
        ":wal",
        ":prometheus",
        "@lz4stream//:lz4stream",
        "@gtest//:gtest_main",
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
