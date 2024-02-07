cc_library(
    name = "boost_1.82.0",
    hdrs = glob([
        "libs/**/include/**/*.h",
        "libs/**/include/**/*.hpp",
        "libs/**/include/**/*.ipp"
    ]),
    includes = glob(
        ["libs/**/include"],
        exclude=[],
        exclude_directories=0,
        allow_empty=False
    ),
    visibility = ["//visibility:public"],
)