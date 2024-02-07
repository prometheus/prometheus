cc_library(
    name = "lru_cache",
    deps = ["//lru_cache:lru_cache"],
    visibility = ["//visibility:public"],
)

cc_library(
    name = "dynamic_lru_cache",
    deps = ["//lru_cache:dynamic_lru_cache"],
    visibility = ["//visibility:public"],
)

cc_test(
    name = "lru_cache_test",
    srcs = ["//lru_cache:tests"],
    deps = [
        ":lru_cache",
        "@gtest//:gtest_main",
    ],
    visibility = ["//visibility:public"],
)