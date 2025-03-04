package(default_visibility = ["//visibility:public"])

cc_binary(
    name = "stream_v_byte_sequence",
    srcs = ["stream_v_byte_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:bare_bones_headers",
        "@google_benchmark//:benchmark_main",
    ],
)
