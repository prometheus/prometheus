package(default_visibility = ["//visibility:public"])

cc_binary(
    name = "generate_reverse_index",
    srcs = ["generate_reverse_index_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:performance_tests_headers",
        "@google_benchmark//:benchmark_main",
    ],
)
