# Description:
#   Jemalloc library

load("@rules_foreign_cc//foreign_cc:configure.bzl", "configure_make")


licenses(["notice"])  # LGPL license

exports_files(["COPYING"])

filegroup(
    name = "src",
    srcs = glob([
        "**",
    ]),
    visibility = ["//visibility:public"],
)

configure_make(
    name = "make_jemalloc",
    autoconf = True,
    configure_in_place = True,
    lib_source = ":src",
    configure_options = [
        "--disable-shared",
        "--enable-static",
    ],
    out_static_libs = [
        "libjemalloc.a",
    ],
    args = ["-j `nproc`"],
    visibility = ["//visibility:public"],
)

cc_library(
    name = "jemalloc",
    visibility = ["//visibility:public"],
    deps = [":make_jemalloc"],
)
