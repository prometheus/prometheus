package(default_visibility = ["//visibility:public"])

cc_library(
    name = "protozero",
    hdrs = glob(["protozero/*.hpp"]),
)

cc_library(
    name = "xxhash",
    hdrs = ["xxhash/xxhash.h"],
)

cc_library(
    name = "uuid",
    hdrs = ["uuid.h"],
)

cc_library(
    name = "third_party",
    deps = [
        ":protozero",
        ":xxhash",
        ":uuid",
    ],
)
