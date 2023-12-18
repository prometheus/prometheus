load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")
load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

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
    url = "https://github.com/bazelbuild/rules_foreign_cc/archive/51152aac9d6d8b887802a47ec08a1a37ef2c4885.zip",
    # sha256 = "a4a7c3a39e90677c78663a07e6d2a4a7ce8529393464f9c0ba51d2ebe5dd8be5",
    sha256 = "2d4a7a935226cf9c01bda163bec54192bc6ffa4a68452794fbea918312c563f4",
    strip_prefix = "rules_foreign_cc-51152aac9d6d8b887802a47ec08a1a37ef2c4885",
)

load("@rules_foreign_cc//foreign_cc:repositories.bzl", "rules_foreign_cc_dependencies")

rules_foreign_cc_dependencies(
    register_built_tools = False,
)

git_repository(
    name = "gtest",
    remote = "https://github.com/google/googletest",
    commit = "58d77fa8070e8cec2dc1ed015d66b454c8d78850",
    shallow_since = "1656350095 -0400",
)

git_repository(
    name = "bazel_clang_tidy",
    remote = "https://github.com/erenon/bazel_clang_tidy.git",
    commit = "11541864afa832ff6721e479c44794e9c9497ae8",
    shallow_since = "1696427391 +0200"
)

http_archive(
    name = "jemalloc",
    url = "https://github.com/jemalloc/jemalloc/releases/download/5.3.0/jemalloc-5.3.0.tar.bz2",  # 5.3
    sha256 = "2db82d1e7119df3e71b7640219b6dfe84789bc0537983c3b7ac4f7189aecfeaa",
    strip_prefix = "jemalloc-5.3.0/",
    patch_args = ["-p1"],
    patches = [
        "//third_party/patches/jemalloc:musl-noexcept-fix.patch",
    ],
    build_file = "//third_party:jemalloc.BUILD",
)

http_archive(
    name = "parallel_hashmap",
    url = "https://github.com/greg7mdp/parallel-hashmap/archive/refs/tags/1.35.zip",
    sha256 = "b61435437713e2d98ce2a5539a0bff7e6e9e6a6b9fe507dbf490a852b8c2904f",
    strip_prefix = "parallel-hashmap-1.35",
    build_file = "//third_party:parallel_hashmap.BUILD",
)

http_archive(
    name = "scope_exit",
    url = "https://github.com/PeterSommerlad/SC22WG21_Papers/archive/ae297346379655dc6ffe306a3f8b133fa0b052c4.zip",
    sha256 = "f5d812f3668cf53e7317213fc68b740a6abe4fd3ddbad7e08c6ed80c51b28828",
    strip_prefix = "SC22WG21_Papers-ae297346379655dc6ffe306a3f8b133fa0b052c4/workspace/P0052_scope_exit/src",
    build_file = "//third_party:scope_exit.BUILD",
)

http_archive(
    name = "lz4",
    url = "https://github.com/lz4/lz4/archive/v1.9.2.tar.gz",
    sha256 = "658ba6191fa44c92280d4aa2c271b0f4fbc0e34d249578dd05e50e76d0e5efcc",
    strip_prefix = "lz4-1.9.2",
    patch_cmds = [
        """sed -i.bak 's/__attribute__ ((__visibility__ ("default")))//g' lib/lz4frame.h """,
    ],
    build_file = "//third_party:lz4.BUILD",
)

http_archive(
    name = "lz4stream",
    url = "https://github.com/laudrup/lz4_stream/archive/6b015cbe786291733b6f3aa03e45d307bc4ae527.zip",
    sha256 = "885cee12f6c37608f6790b5fbd856e4b0fcfb8637eeba299e1d888ff2cdc9f31",
    strip_prefix = "lz4_stream-6b015cbe786291733b6f3aa03e45d307bc4ae527/include",
    build_file = "//third_party:lz4stream.BUILD",
)

http_archive(
    name = "argp",
    url = "https://github.com/argp-standalone/argp-standalone/archive/refs/tags/1.5.0.tar.gz",
    sha256 = "c29eae929dfebd575c38174f2c8c315766092cec99a8f987569d0cad3c6d64f6",
    strip_prefix = "argp-standalone-1.5.0/",
    patch_args = ["-p1"],
    patches = [
        "//third_party/patches/argp:0001-Makefile.am.patch",
    ],
    build_file = "//third_party:argp.BUILD",
)

http_archive(
    name = "fts",
    url = "https://github.com/void-linux/musl-fts/archive/refs/tags/v1.2.7.tar.gz",
    sha256 = "49ae567a96dbab22823d045ffebe0d6b14b9b799925e9ca9274d47d26ff482a6",
    strip_prefix = "musl-fts-1.2.7/",
    build_file = "//third_party:fts.BUILD",
)

http_archive(
    name = "obstack",
    url = "https://github.com/void-linux/musl-obstack/archive/refs/tags/v1.2.3.tar.gz",
    sha256 = "9ffb3479b15df0170eba4480e51723c3961dbe0b461ec289744622db03a69395",
    strip_prefix = "musl-obstack-1.2.3/",
    build_file = "//third_party:obstack.BUILD",
)

http_archive(
    name = "zlib",
    url = "https://zlib.net/zlib-1.3.tar.gz",
    sha256 = "ff0ba4c292013dbc27530b3a81e1f9a813cd39de01ca5e0f8bf355702efa593e",
    strip_prefix = "zlib-1.3/",
    build_file = "//third_party:zlib.BUILD",
)

http_archive(
    name = "zstd",
    url = "https://github.com/facebook/zstd/releases/download/v1.5.5/zstd-1.5.5.tar.gz",
    sha256 = "9c4396cc829cfae319a6e2615202e82aad41372073482fce286fac78646d3ee4",
    strip_prefix = "zstd-1.5.5/",
    build_file = "//third_party:zstd.BUILD",
)

http_archive(
    name = "lzma",
    url = "https://tukaani.org/xz/xz-5.4.4.tar.xz",
    sha256 = "705d0d96e94e1840e64dec75fc8d5832d34f6649833bec1ced9c3e08cf88132e",
    strip_prefix = "xz-5.4.4/",
    build_file = "//third_party:lzma.BUILD",
)

local_repository(
    name = "musl-legacy-error",
    path = "third_party/musl-legacy-error",
)

http_archive(
    name = "elf",
    url = "https://sourceware.org/elfutils/ftp/0.189/elfutils-0.189.tar.bz2",
    sha256 = "39bd8f1a338e2b7cd4abc3ff11a0eddc6e690f69578a57478d8179b4148708c8",
    strip_prefix = "elfutils-0.189/",
    patch_args = ["-p1"],
    patches = [
        "//third_party/patches/elf:0001-fix-aarch64_fregs.patch",
        "//third_party/patches/elf:0002-fix-uninitialized.patch",
        "//third_party/patches/elf:0003-musl-macros.patch",
        "//third_party/patches/elf:0004-Makefile.in.patch",
    ],
    build_file = "//third_party:elf.BUILD",
)

http_archive(
    name = "backward_cpp",
    url = "https://github.com/bombela/backward-cpp/archive/65a769ffe77cf9d759d801bc792ac56af8e911a3.zip",
    sha256 = "fa6e7d2919eca555772ceb9a946e58d95cb562cb01bb59d01f901f607656cd32",
    strip_prefix = "backward-cpp-65a769ffe77cf9d759d801bc792ac56af8e911a3/",
    build_file = "//third_party:backward_cpp.BUILD",
)

http_archive(
    name = "roaring",
    url = "https://github.com/RoaringBitmap/CRoaring/archive/refs/tags/v1.3.0.zip",
    sha256 = "a037e12a3f7c8c2abb3e81fc9669c23e274ffa2d8670d2034a2e05969e53689b",
    strip_prefix = "CRoaring-1.3.0/",
    patch_args = ["-p1"],
    patches = [
        "//third_party/patches/roaring:0001-disable-test-dependencies.patch",
    ],
    build_file = "//third_party:roaring.BUILD",
)
