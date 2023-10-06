load("@bazel_tools//tools/build_defs/cc:action_names.bzl", "ACTION_NAMES")
load(
    "@bazel_tools//tools/cpp:cc_toolchain_config_lib.bzl",
    "feature",
    "flag_group",
    "flag_set",
    "tool_path",
    "with_feature_set",
)
load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")

all_compile_actions = [
    ACTION_NAMES.assemble,
    ACTION_NAMES.c_compile,
    ACTION_NAMES.clif_match,
    ACTION_NAMES.cpp_compile,
    ACTION_NAMES.cpp_header_parsing,
    ACTION_NAMES.cpp_module_codegen,
    ACTION_NAMES.cpp_module_compile,
    ACTION_NAMES.linkstamp_compile,
    ACTION_NAMES.lto_backend,
    ACTION_NAMES.preprocess_assemble,
]

cpp_compile_actions = [
    ACTION_NAMES.cpp_compile,
    ACTION_NAMES.cpp_header_parsing,
    ACTION_NAMES.cpp_module_codegen,
    ACTION_NAMES.cpp_module_compile,
]

all_link_actions = [
    ACTION_NAMES.cpp_link_executable,
    ACTION_NAMES.cpp_link_dynamic_library,
    ACTION_NAMES.cpp_link_nodeps_dynamic_library,
]

def _impl(ctx):
    is_in_musl_container = ctx.attr.is_in_musl_container[BuildSettingInfo].value
    tool_paths = [
        tool_path(
            name = "gcc",
            path = "/opt/bin/gcc" if is_in_musl_container else "/usr/bin/gcc-12",
        ),
        tool_path(
            name = "g++",
            path = "/opt/bin/g++" if is_in_musl_container else "/usr/bin/g++-12",
        ),
        tool_path(
            name = "ld",
            path = "/opt/bin/ld" if is_in_musl_container else "/usr/bin/ld",
        ),
        tool_path(
            name = "ar",
            path = "/opt/bin/ar" if is_in_musl_container else "/usr/bin/ar",
        ),
        tool_path(
            name = "cpp",
            path = "/bin/false",
        ),
        tool_path(
            name = "gcov",
            path = "/bin/false",
        ),
        tool_path(
            name = "nm",
            path = "/bin/false",
        ),
        tool_path(
            name = "objdump",
            path = "/bin/false",
        ),
        tool_path(
            name = "strip",
            path = "/bin/false",
        ),
    ]

    features = [
        feature(
            name = "dbg",
        ),
        feature(
            name = "opt",
        ),
        feature(
            name = "default_compiler_flags",
            enabled = True,
            flag_sets = [
                flag_set(
                    actions = all_compile_actions,
                    flag_groups = [
                        flag_group(
                            flags = [
                                # Common compile flags here
                                "-fPIC",
                                "-Wall",
                                "-gdwarf",
                                "-ggdb",
                                "-g",
                            ],
                        ),
                    ],
                ),
                flag_set(
                    actions = cpp_compile_actions,
                    flag_groups = [
                        flag_group(
                            flags = [
                                # Common compile flags here
                                "-std=c++2b",
                            ],
                        ),
                    ],
                ),
                flag_set(
                    actions = all_compile_actions,
                    flag_groups = [
                        flag_group(
                            flags = [
                                # Additional flags for "-c dbg"
                                "-g",
                            ],
                        ),
                    ],
                    with_features = [
                        with_feature_set(
                            features = [
                                "dbg",
                            ],
                        ),
                    ],
                ),
                flag_set(
                    actions = all_compile_actions,
                    flag_groups = [
                        flag_group(
                            flags = [
                                # Additional flags for "-c opt"
                                "-O3",
                                "-DNDEBUG",
                            ] + ctx.attr.cpu_compiler_options,
                        ),
                    ],
                    with_features = [
                        with_feature_set(
                            features = [
                                "opt",
                            ],
                        ),
                    ],
                ),
                flag_set(
                    actions = cpp_compile_actions,
                    flag_groups = [
                        flag_group(
                            flags = [
                                # Additional flags for "-c opt" (C++ only)
                                "-fconcepts-diagnostics-depth=4",
                                "-finline-limit=10000",
                                "--param=large-function-insns=20000",
                            ],
                        ),
                    ],
                    with_features = [
                        with_feature_set(
                            features = [
                                "opt",
                            ],
                        ),
                    ],
                ),
            ],
        ),
        feature(
            name = "default_linker_flags",
            enabled = True,
            flag_sets = [
                flag_set(
                    actions = all_link_actions,
                    flag_groups = ([
                        flag_group(
                            flags = [
                                "-static-libgcc",
                                "-static-libstdc++",
                                "-l:libstdc++.a",
                                "-l:libm.a",
                            ],
                        ),
                    ]),
                ),
            ],
        ),
    ]
    return cc_common.create_cc_toolchain_config_info(
        ctx = ctx,
        toolchain_identifier = "local",
        host_system_name = "local",
        target_system_name = "local",
        target_cpu = "k8",
        target_libc = "unknown",
        compiler = "g++",
        abi_version = "unknown",
        abi_libc_version = "unknown",
        tool_paths = tool_paths,
        features = features,
        cxx_builtin_include_directories = ctx.attr.builtin_include_directories,
    )

cc_toolchain_config = rule(
    implementation = _impl,
    attrs = {
        "builtin_include_directories": attr.string_list(),
        "cpu_compiler_options": attr.string_list(),
        "is_in_musl_container": attr.label(),
    },
    provides = [CcToolchainConfigInfo],
)
