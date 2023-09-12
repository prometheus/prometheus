load(":cc_toolchain_config.bzl", "cc_toolchain_config")

package(default_visibility = ["//visibility:public"])

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
    builtin_include_directories = [
        "/usr/lib/gcc/x86_64-linux-gnu/12",
        "/usr/include",
    ],
    cpu_compiler_options = [
        "-march=znver2",
    ],
)

cc_toolchain_config(
    name = "g++-12_toolchain_config_aarch64",
    builtin_include_directories = [
        "/usr/lib/gcc/aarch64-linux-gnu/12",
        "/usr/include",
    ],
    cpu_compiler_options = [
	"-march=armv8-a+crc"
    ],
)
