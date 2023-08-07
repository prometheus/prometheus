load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")
load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive", "http_file")

git_repository(
    name = "gtest",
    remote = "https://github.com/google/googletest",
    commit = "58d77fa8070e8cec2dc1ed015d66b454c8d78850",
    shallow_since = "1656350095 -0400",
)

git_repository(
    name = "bazel_clang_tidy",
    commit = "783aa523aafb4a6798a538c61e700b6ed27975a7",
    shallow_since = "1620650326 +0800",
    remote = "https://github.com/erenon/bazel_clang_tidy.git",
)

http_archive(
    name = "jemalloc",
    url = "https://github.com/jemalloc/jemalloc/archive/20f9802e4f25922884448d9581c66d76cc905c0c.zip",  # 5.3
    sha256 = "1cc1ec93701868691c73b371eb87e5452257996279a42303a91caad355374439",
    build_file = "//third_party:jemalloc.BUILD",
    strip_prefix = "jemalloc-20f9802e4f25922884448d9581c66d76cc905c0c",
)

http_archive(
    name = "parallel_hashmap",
    url = "https://github.com/greg7mdp/parallel-hashmap/archive/refs/tags/1.35.zip",
    sha256 = "b61435437713e2d98ce2a5539a0bff7e6e9e6a6b9fe507dbf490a852b8c2904f",
    build_file = "//third_party:parallel_hashmap.BUILD",
    strip_prefix = "parallel-hashmap-1.35",
)

http_archive(
    name = "scope_exit",
    url = "https://github.com/PeterSommerlad/SC22WG21_Papers/archive/ae297346379655dc6ffe306a3f8b133fa0b052c4.zip",
    sha256 = "f5d812f3668cf53e7317213fc68b740a6abe4fd3ddbad7e08c6ed80c51b28828",
    build_file = "//third_party:scope_exit.BUILD",
    strip_prefix = "SC22WG21_Papers-ae297346379655dc6ffe306a3f8b133fa0b052c4/workspace/P0052_scope_exit/src",
)

http_archive(
    name = "lz4",
    url = "https://github.com/lz4/lz4/archive/v1.9.2.tar.gz",
    sha256 = "658ba6191fa44c92280d4aa2c271b0f4fbc0e34d249578dd05e50e76d0e5efcc",
    build_file = "//third_party:lz4.BUILD",
    strip_prefix = "lz4-1.9.2",
    patch_cmds = [
        """sed -i.bak 's/__attribute__ ((__visibility__ ("default")))//g' lib/lz4frame.h """,
    ],
)

http_archive(
    name = "lz4stream",
    url = "https://github.com/laudrup/lz4_stream/archive/6b015cbe786291733b6f3aa03e45d307bc4ae527.zip",
    sha256 = "885cee12f6c37608f6790b5fbd856e4b0fcfb8637eeba299e1d888ff2cdc9f31",
    build_file = "//third_party:lz4stream.BUILD",
    strip_prefix = "lz4_stream-6b015cbe786291733b6f3aa03e45d307bc4ae527/include",
)

http_archive(
    name = "backward_cpp",
    url = "https://github.com/bombela/backward-cpp/archive/refs/heads/master.zip",
    sha256 = "7c93aba757cdbf835ee29e65e6e50df7a9ceabb7bdb21dc7ad9a42058220afa8",
    build_file = "//third_party:backward_cpp.BUILD",
    strip_prefix = "backward-cpp-master/",
)
