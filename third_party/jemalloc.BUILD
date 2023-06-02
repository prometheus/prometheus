# There's a bunch of workarounds in this build command.
# - We workaround https://github.com/bazelbuild/bazel/issues/3128
# - We need to convert CC et al to absolute paths, but we can't use
#   `realpath` because we need to run them out of a workspace-relative
#   path for our CROSSTOOL to find clang.
# - CC et al are shell-script wrappers on macOS, and we need to add an
#   explicit `sh ...` for reasons we don't fully understand.
# - We have to set AR on Linux, but on macOS that points to `libtool`
#   which has a different calling convention, so we leave it unset
#   there and let configure find the default `ar`
JEMALLOC_BUILD_COMMAND = """
  pushd $$(dirname $(location autogen.sh))
  ./autogen.sh --disable-shared --enable-static 2>&1 && make build_lib_static -j$$(nproc) 2>&1
  popd
  mv $$(dirname $(location autogen.sh))/lib/libjemalloc.a $(location lib/libjemalloc.a)
  mv $$(dirname $(location autogen.sh))/include/jemalloc/jemalloc.h $(location include/jemalloc/jemalloc.h)
"""

genrule(
    name = "jemalloc_genrule",
    srcs = glob(
        [
            "**/*.cc",
            "**/*.c",
            "**/*.cpp",
            "**/*.h",
            "**/*.in",
            "**/*.sh",
            "**/*.ac",
            "**/*.m4",
            "**/*.guess",
            "**/*.sub",
            "**/install-sh",
        ],
        exclude = [
            "include/jemalloc/jemalloc.h",
        ],
    ),
    outs = [
        "lib/libjemalloc.a",
        "include/jemalloc/jemalloc.h",
    ],
    cmd = JEMALLOC_BUILD_COMMAND,
    toolchains = [
        "@bazel_tools//tools/cpp:cc_flags",
        "@bazel_tools//tools/cpp:current_cc_toolchain",
    ],
)

cc_library(
    name = "jemalloc",
    srcs = [":jemalloc_genrule"],
    hdrs = ["include/jemalloc/jemalloc.h"],
    linkopts = [
        "-ldl"
    ],
    linkstatic = 1,
    visibility = ["//visibility:public"],
)
