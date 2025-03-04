package(default_visibility = ["//visibility:public"])

cc_binary(
    name = "scraper",
    srcs = ["scraper_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:wal",
        "@google_benchmark//:benchmark_main",
    ],
)
