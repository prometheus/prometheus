# Description:
#   LZ4 library

licenses(["notice"])  # BSD license

exports_files(["LICENSE"])

cc_library(
    name = "lz4",
    srcs = [
        "lib/lz4.c",
        "lib/lz4frame.c",
        "lib/lz4hc.c",
    ],
    hdrs = [
        "lib/lz4.h",
        "lib/lz4frame.h",
        "lib/lz4hc.h",
        "lib/xxhash.h",
    ],
    defines = [
        "XXH_PRIVATE_API",
    ],
    includes = [
        "lib",
    ],
    textual_hdrs = [
        "lib/xxhash.c",
        "lib/lz4.c",
    ],
    visibility = ["//visibility:public"],
)
