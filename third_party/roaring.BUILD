load("@rules_foreign_cc//foreign_cc:cmake.bzl", "cmake")

filegroup(
    name = "src",
    srcs = glob([
        "**",
    ]),
    visibility = ["//visibility:public"],
)

cmake(
    name = "roaring",
    lib_source = ":src",
    generate_args = [
        "-DENABLE_ROARING_TESTS=OFF",
    ],
    build_args = ["-j `nproc`"],
    out_static_libs = [
        "libroaring.a",
    ],
    visibility = ["//visibility:public"],
)
