package(default_visibility = ["//visibility:public"])

cc_binary(
    name = "series_data_encoder",
    srcs = ["series_data_encoder_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:series_data",
        "@google_benchmark//:benchmark_main",
    ],
)