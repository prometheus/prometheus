load("@bazel_tools//tools/cpp:toolchain_utils.bzl", "find_cpp_toolchain")
load("@bazel_tools//tools/build_defs/cc:action_names.bzl", "C_COMPILE_ACTION_NAME")

def _impl(ctx):
    cc_toolchain = find_cpp_toolchain(ctx)
    feature_configuration = cc_common.configure_features(
        ctx = ctx,
        cc_toolchain = cc_toolchain,
        requested_features = ctx.features,
        unsupported_features = ctx.disabled_features,
    )
    cc_path = cc_common.get_tool_for_action(
        feature_configuration = feature_configuration,
        action_name = C_COMPILE_ACTION_NAME,
    )

    in_file = ctx.file.entrypoint
    out_file = ctx.outputs.out

    module_path = ctx.build_file_path[:-6] # remove '/BUILD'

    ld_flags = []
    lib_files = []
    for d in ctx.attr.deps:
        for f in d.output_groups.archive.to_list():
            lib_files.append(f)
            lib = f.basename[3:-2]
            ld_flags.append('-L ../%s -l%s' % (f.dirname, lib))

    ctx.actions.run_shell(
        inputs = ctx.files.srcs + lib_files,
        outputs = [ctx.outputs.out],
        env = {
            'CGO_CC': cc_path,
            'PATH': '/usr/bin',
            'CGO_LDFLAGS': '-lstdc++ ' + ' '.join(ld_flags),
            'HOME': '$PWD/..',
        },
        arguments = [module_path, in_file.path, out_file.path],
        command = 'cd $1 && go build -mod=vendor -o "../$3" "../$2"',
    )

go_binary = rule(
    implementation = _impl,
    attrs = {
        "entrypoint": attr.label(
            allow_single_file = True,
            mandatory = True,
        ),
        "srcs": attr.label_list(allow_files = True),
        "out": attr.output(mandatory = True),
        "deps": attr.label_list(),
        "_cc_toolchain": attr.label(default = "@bazel_tools//tools/cpp:current_cc_toolchain"),
    },
    fragments = ["cpp"],
)
