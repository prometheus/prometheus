load(":cc_toolchain_config.bzl", "cc_toolchain_config")
load("@bazel_skylib//rules:common_settings.bzl", "bool_flag", "string_flag")

package(default_visibility = ["//visibility:public"])

config_setting(
    name = "arm64",
    values = {"cpu": "arm"},
)

config_setting(
    name = "amd64",
    values = {"cpu": "x86"},
)

bool_flag(
    name = "asan",
    build_setting_default = 0,
)

config_setting(
    name = "with_asan",
    flag_values = {"asan": "1"},
)

string_flag(
    name = "march",
    build_setting_default = "native",
)

cc_toolchain_suite(
    name = "g++-12",
    toolchains = {
        "k8": ":g++-12_toolchain_x86_64",
        "aarch64": ":g++-12_toolchain_aarch64",
    },
)

filegroup(name = "empty")

cc_toolchain(
    name = "g++-12_toolchain_x86_64",
    all_files = ":empty",
    compiler_files = ":empty",
    dwp_files = ":empty",
    linker_files = ":empty",
    objcopy_files = ":empty",
    strip_files = ":empty",
    supports_param_files = 0,
    toolchain_config = ":g++-12_toolchain_config_x86_64",
    toolchain_identifier = "g++-12_toolchain",
)

cc_toolchain(
    name = "g++-12_toolchain_aarch64",
    all_files = ":empty",
    compiler_files = ":empty",
    dwp_files = ":empty",
    linker_files = ":empty",
    objcopy_files = ":empty",
    strip_files = ":empty",
    supports_param_files = 0,
    toolchain_config = ":g++-12_toolchain_config_aarch64",
    toolchain_identifier = "g++-12_toolchain",
)

cc_toolchain_config(
    name = "g++-12_toolchain_config_x86_64",
    builtin_include_directories = [
        "/usr/lib/gcc/x86_64-alpine-linux-musl",
        "/usr/lib/gcc/x86_64-linux-gnu/12",
        "/usr/include",
    ],
    march = ":march",
    with_asan = ":asan",
)

cc_toolchain_config(
    name = "g++-12_toolchain_config_aarch64",
    builtin_include_directories = [
        "/usr/lib/gcc/aarch64-alpine-linux-musl/12.2.1/include/",
        "/usr/lib/gcc/aarch64-linux-gnu/12",
        "/usr/include",
    ],
    march = ":march",
    with_asan = ":asan",
)
