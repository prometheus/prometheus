workspace(name = "prompp")

load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")
load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_file")

http_archive(
    name = "bazel_skylib",
    sha256 = "66ffd9315665bfaafc96b52278f57c7e2dd09f5ede279ea6d39b2be471e7e3aa",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/bazel-skylib/releases/download/1.4.2/bazel-skylib-1.4.2.tar.gz",
        "https://github.com/bazelbuild/bazel-skylib/releases/download/1.4.2/bazel-skylib-1.4.2.tar.gz",
    ],
)

load("@bazel_skylib//:workspace.bzl", "bazel_skylib_workspace")

bazel_skylib_workspace()

http_archive(
    name = "rules_foreign_cc",
    # sha256 = "a4a7c3a39e90677c78663a07e6d2a4a7ce8529393464f9c0ba51d2ebe5dd8be5",
    sha256 = "2d4a7a935226cf9c01bda163bec54192bc6ffa4a68452794fbea918312c563f4",
    strip_prefix = "rules_foreign_cc-51152aac9d6d8b887802a47ec08a1a37ef2c4885",
    url = "https://github.com/bazelbuild/rules_foreign_cc/archive/51152aac9d6d8b887802a47ec08a1a37ef2c4885.zip",
)

load("@rules_foreign_cc//foreign_cc:repositories.bzl", "rules_foreign_cc_dependencies")

rules_foreign_cc_dependencies(
    register_built_tools = False,
)

git_repository(
    name = "gtest",
    commit = "58d77fa8070e8cec2dc1ed015d66b454c8d78850",
    patches = [
        "//third_party/patches/gtest:no-werror.patch",
    ],
    remote = "https://github.com/google/googletest",
    shallow_since = "1656350095 -0400",
)

http_archive(
    name = "google_benchmark",
    patches = [
        "//third_party/patches/google_benchmark:BUILD.bazel.patch"
    ],
    sha256 = "35a77f46cc782b16fac8d3b107fbfbb37dcd645f7c28eee19f3b8e0758b48994",
    strip_prefix = "benchmark-1.9.0/",
    url = "https://github.com/google/benchmark/archive/refs/tags/v1.9.0.tar.gz",
)

git_repository(
    name = "bazel_clang_tidy",
    commit = "11541864afa832ff6721e479c44794e9c9497ae8",
    remote = "https://github.com/erenon/bazel_clang_tidy.git",
    shallow_since = "1696427391 +0200",
)

http_archive(
    name = "jemalloc",
    build_file = "//third_party:jemalloc.BUILD",
    patch_args = ["-p1"],
    patches = [
        "//third_party/patches/jemalloc:0001-musl-noexcept-fix.patch",
        "//third_party/patches/jemalloc:0002-manual-init.patch",
    ],
    sha256 = "2db82d1e7119df3e71b7640219b6dfe84789bc0537983c3b7ac4f7189aecfeaa",
    strip_prefix = "jemalloc-5.3.0/",
    url = "https://github.com/jemalloc/jemalloc/releases/download/5.3.0/jemalloc-5.3.0.tar.bz2",  # 5.3
)

http_archive(
    name = "parallel_hashmap",
    build_file = "//third_party:parallel_hashmap.BUILD",
    patches = [
        "//third_party/patches/parallel_hashmap:phmap_base.h.patch",
    ],
    sha256 = "b61435437713e2d98ce2a5539a0bff7e6e9e6a6b9fe507dbf490a852b8c2904f",
    strip_prefix = "parallel-hashmap-1.35",
    url = "https://github.com/greg7mdp/parallel-hashmap/archive/refs/tags/1.35.zip",
)

http_archive(
    name = "scope_exit",
    build_file = "//third_party:scope_exit.BUILD",
    sha256 = "f5d812f3668cf53e7317213fc68b740a6abe4fd3ddbad7e08c6ed80c51b28828",
    strip_prefix = "SC22WG21_Papers-ae297346379655dc6ffe306a3f8b133fa0b052c4/workspace/P0052_scope_exit/src",
    url = "https://github.com/PeterSommerlad/SC22WG21_Papers/archive/ae297346379655dc6ffe306a3f8b133fa0b052c4.zip",
)

http_archive(
    name = "lz4",
    build_file = "//third_party:lz4.BUILD",
    patches = [
        "//third_party/patches/lz4:lz4frame.h.patch",
        "//third_party/patches/lz4:lz4frame.c.patch",
        "//third_party/patches/lz4:lz4hc.c.patch",
    ],
    sha256 = "658ba6191fa44c92280d4aa2c271b0f4fbc0e34d249578dd05e50e76d0e5efcc",
    strip_prefix = "lz4-1.9.2",
    url = "https://github.com/lz4/lz4/archive/v1.9.2.tar.gz",
)

http_archive(
    name = "roaring",
    build_file = "//third_party:roaring.BUILD",
    patch_args = ["-p1"],
    patches = [
        "//third_party/patches/roaring:0001-disable-test-dependencies.patch",
    ],
    sha256 = "a037e12a3f7c8c2abb3e81fc9669c23e274ffa2d8670d2034a2e05969e53689b",
    strip_prefix = "CRoaring-1.3.0/",
    url = "https://github.com/RoaringBitmap/CRoaring/archive/refs/tags/v1.3.0.zip",
)

http_archive(
    name = "boost_1.82.0",
    build_file = "//third_party:boost_1.82.0.BUILD",
    sha256 = "b62bd839ea6c28265af9a1f68393eda37fab3611425d3b28882d8e424535ec9d",
    strip_prefix = "boost-1.82.0/",
    url = "https://github.com/boostorg/boost/releases/download/boost-1.82.0/boost-1.82.0.tar.gz",
    patch_cmds = [
        # Remove directories with `build` name
        # Workaround for https://github.com/bazelbuild/bazel/issues/22484
        "find ./libs -type d -wholename '*/build' | xargs rm -rf"
    ]
)

http_archive(
    name = "xxHash",
    build_file = "//third_party:xxHash.BUILD",
    sha256 = "aae608dfe8213dfd05d909a57718ef82f30722c392344583d3f39050c7f29a80",
    strip_prefix = "xxHash-0.8.3/",
    url = "https://github.com/Cyan4973/xxHash/archive/refs/tags/v0.8.3.tar.gz",
)

http_archive(
    name = "ceph_fmt",
    build_file = "//third_party:ceph/fmt.BUILD",
    sha256 = "5dea48d1fcddc3ec571ce2058e13910a0d4a6bab4cc09a809d8b1dd1c88ae6f2",
    strip_prefix = "fmt-9.1.0/",
    url = "https://github.com/ceph/fmt/archive/refs/tags/9.1.0.tar.gz",
)

http_archive(
    name = "ceph",
    build_file = "//third_party:ceph/ceph.BUILD",
    patch_args = ["-p1"],
    patches = [
        "//third_party/patches/ceph:needed_includes_for_build.patch",
    ],
    sha256 = "5b9d53b038bb050f123070e903bebedfd4a19df63af2f8ba348f3f2c09ce2ecc",
    strip_prefix = "ceph-17.2.7/",
    url = "https://github.com/ceph/ceph/archive/refs/tags/v17.2.7.tar.gz",
)

http_archive(
    name = "com_google_absl",
    patches = [
        "//third_party/patches/com_google_absl:no-werror.patch",
    ],
    sha256 = "f8903111260a18d2cc4618cd5bf35a22bcc28f372ebe4f04024b49e88a2e16c1",
    strip_prefix = "abseil-cpp-20240116.rc1/",
    url = "https://github.com/abseil/abseil-cpp/archive/refs/tags/20240116.rc1.tar.gz",
)

local_repository(
    name = "lru_cache",
    path = "third_party/lru_cache",
)

git_repository(
    name = "snappy",
    commit = "27f34a580be4a3becf5f8c0cba13433f53c21337",
    remote = "https://github.com/google/snappy",
    shallow_since = "1689185568 -0700",
)

http_archive(
    name = "re2",
    patches = [
        "//third_party/patches/re2:no-werror.patch",
    ],
    sha256 = "cd191a311b84fcf37310e5cd876845b4bf5aee76fdd755008eef3b6478ce07bb",
    strip_prefix = "re2-2024-02-01/",
    url = "https://github.com/google/re2/archive/refs/tags/2024-02-01.tar.gz",
)

git_repository(
    name = "cedar",
    build_file = "//third_party:cedar.BUILD",
    commit = "38fa7f615f14bf867834c796945841d54cb45e0f",
    patch_args = ["-p1"],
    patches = [
        "//third_party/patches/cedar:cedarpp.h.patch",
    ],
    remote = "https://github.com/DevO2012/cedar",
)

git_repository(
    name = "quasis_crypto",
    build_file = "//third_party:quasis_crypto.BUILD",
    commit = "7d3c4c648b1013e37d25d247c31078033b10d172",
    patch_args = ["-p1"],
    patches = [
        "//third_party/patches/quasis_crypto:md5.hh.patch",
    ],
    remote = "https://github.com/quasis/crypto",
)

http_archive(
    name = "simdutf",
    build_file = "//third_party:simdutf.BUILD",
    sha256 = "66c85f591133e3baa23cc441d6e2400dd2c94c4902820734ddbcd9e04dd3988b",
    url = "https://github.com/simdutf/simdutf/releases/download/v6.2.0/singleheader.zip",
)

http_file(
    name = "fastfloat_header",
    downloaded_file_path = "fastfloat/fast_float.h",
    url = "https://github.com/fastfloat/fast_float/releases/download/v8.0.0/fast_float.h",
    sha256 = "1335e82c61fda54476ecbd94b92356deebeb3f0122802c3f103ee528ac08624e",
)

local_repository(
    name = "fastfloat",
    path = "third_party/fastfloat",
)