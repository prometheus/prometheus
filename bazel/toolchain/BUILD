load("@bazel_skylib//rules:common_settings.bzl", "bool_flag")
load(":cc_toolchain_config.bzl", "cc_toolchain_config")
package(default_visibility = ["//visibility:public"])

bool_flag(
    name = "in_musl_container",
    build_setting_default = False,
)

config_setting(
    name = "is_in_musl_container",
    flag_values = {
        ":in_musl_container": "True",
    },
)

cc_toolchain_suite(
    name = "g++-12",
    toolchains = {
        "k8": ":g++-12_toolchain_x86_64",
        "aarch64": ":g++-12_toolchain_aarch64"
    },
)

filegroup(name = "empty")

cc_toolchain(
    name = "g++-12_toolchain_x86_64",
    toolchain_identifier = "g++-12_toolchain",
    toolchain_config = ":g++-12_toolchain_config_x86_64",
    all_files = ":empty",
    compiler_files = ":empty",
    dwp_files = ":empty",
    linker_files = ":empty",
    objcopy_files = ":empty",
    strip_files = ":empty",
    supports_param_files = 0,
)

cc_toolchain(
    name = "g++-12_toolchain_aarch64",
    toolchain_identifier = "g++-12_toolchain",
    toolchain_config = ":g++-12_toolchain_config_aarch64",
    all_files = ":empty",
    compiler_files = ":empty",
    dwp_files = ":empty",
    linker_files = ":empty",
    objcopy_files = ":empty",
    strip_files = ":empty",
    supports_param_files = 0,
)

cc_toolchain_config(
    name = "g++-12_toolchain_config_x86_64",
    builtin_include_directories = select({
        ":is_in_musl_container": [
            "/opt/lib/gcc/x86_64-linux-musl/12.3.0/include/",
            "/opt/x86_64-linux-musl/include/",
        ],
        "//conditions:default": [
            "/usr/lib/gcc/x86_64-linux-gnu/12",
            "/usr/include",
        ],
    }),
    cpu_compiler_options = [
        "-march=znver2",
    ],
    is_in_musl_container = ":in_musl_container",
)

cc_toolchain_config(
    name = "g++-12_toolchain_config_aarch64",
    builtin_include_directories = select({
        ":is_in_musl_container": [
            "/opt/lib/gcc/aarch64-linux-musl/12.3.0/include/",
            "/opt/aarch64-linux-musl/include/",
            "/opt/arm-linux-musleabi/include/",
        ],
        "//conditions:default": [
            "/usr/lib/gcc/aarch64-linux-gnu/12",
            "/usr/include",
        ],
    }),
    cpu_compiler_options = [
        "-march=armv8-a+crc"
    ],
    is_in_musl_container = ":in_musl_container",
)
