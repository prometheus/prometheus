cc_library(
    name = "ceph",
    hdrs = glob([
        "src/**/*.h",
        "src/**/*.hpp",
    ]),
    includes = ["./src/include", "./src/"],
    visibility = ["//visibility:public"],
    deps = [
        "@boost_1.82.0",
        "@ceph_fmt",
        "@xxHash",
    ]
)
