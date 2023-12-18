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
    tool_paths = [
        tool_path(
            name = "gcc",
            path = "/usr/bin/gcc",
        ),
        tool_path(
            name = "g++",
            path = "/usr/bin/g++",
        ),
        tool_path(
            name = "ld",
            path = "/usr/bin/ld",
        ),
        tool_path(
            name = "ar",
            path = "/usr/bin/ar",
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
                                "-march=" + ctx.attr.march[BuildSettingInfo].value,
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
                                # Additional flags for "-c opt"
                                "-O3",
                                "-DNDEBUG",
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
            ]
        ),
        feature(
            name = "sanitizers-address",
            provides = ["sanitizer"],
            enabled = ctx.attr.with_asan[BuildSettingInfo].value,
            flag_sets = [
                flag_set(
                    actions = cpp_compile_actions,
                    flag_groups = [
                        flag_group(
                            flags = [
                                "-fno-omit-frame-pointer",
                                "-fsanitize=address",
                            ],
                        ),
                    ],
                ),
                flag_set(
                    actions = all_link_actions,
                    flag_groups = ([
                        flag_group(
                            flags = [
                                "-fsanitize=address",
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
        "march": attr.label(),
        "with_asan": attr.label(),
    },
    provides = [CcToolchainConfigInfo],
)
