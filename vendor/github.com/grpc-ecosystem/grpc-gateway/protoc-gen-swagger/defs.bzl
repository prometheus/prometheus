"""Generated an open-api spec for a grpc api spec.

Reads the the api spec in protobuf format and generate an open-api spec.
Optionally applies settings from the grpc-service configuration.
"""

def _collect_includes(gen_dir, srcs):
    """Build an include path mapping.

    It is important to not just collect unique dirnames, to also support
    proto files of the same name from different packages.

    The implementation below is similar to what bazel does in its
    ProtoCompileActionBuilder.java
    """
    includes = []
    for src in srcs:
        ref_path = src.path

        if ref_path.startswith(gen_dir):
            ref_path = ref_path[len(gen_dir):].lstrip("/")

        if src.owner.workspace_root:
            workspace_root = src.owner.workspace_root
            ref_path = ref_path[len(workspace_root):].lstrip("/")

        include = ref_path + "=" + src.path
        if include not in includes:
            includes.append(include)

    return includes

def _run_proto_gen_swagger(ctx, direct_proto_srcs, transitive_proto_srcs, actions, protoc, protoc_gen_swagger, grpc_api_configuration, single_output):
    swagger_files = []

    inputs = direct_proto_srcs + transitive_proto_srcs
    tools = [protoc_gen_swagger]

    options = ["logtostderr=true", "allow_repeated_fields_in_body=true"]
    if grpc_api_configuration:
        options.append("grpc_api_configuration=%s" % grpc_api_configuration.path)
        inputs.append(grpc_api_configuration)

    includes = _collect_includes(ctx.genfiles_dir.path, direct_proto_srcs + transitive_proto_srcs)

    if single_output:
        swagger_file = actions.declare_file(
            "%s.swagger.json" % ctx.attr.name,
            sibling = direct_proto_srcs[0],
        )
        output_dir = ctx.bin_dir.path
        if direct_proto_srcs[0].owner.workspace_root:
            output_dir = "/".join([output_dir, direct_proto_srcs[0].owner.workspace_root])

        output_dir = "/".join([output_dir, direct_proto_srcs[0].dirname])

        options.append("allow_merge=true")
        options.append("merge_file_name=%s" % ctx.attr.name)

        args = actions.args()
        args.add("--plugin=%s" % protoc_gen_swagger.path)
        args.add("--swagger_out=%s:%s" % (",".join(options), output_dir))
        args.add_all(["-I%s" % include for include in includes])
        args.add_all([src.path for src in direct_proto_srcs])

        actions.run(
            executable = protoc,
            inputs = inputs,
            tools = tools,
            outputs = [swagger_file],
            arguments = [args],
        )

        swagger_files.append(swagger_file)
    else:
        for proto in direct_proto_srcs:
            swagger_file = actions.declare_file(
                "%s.swagger.json" % proto.basename[:-len(".proto")],
                sibling = proto,
            )

            output_dir = ctx.bin_dir.path
            if proto.owner.workspace_root:
                output_dir = "/".join([output_dir, proto.owner.workspace_root])

            args = actions.args()
            args.add("--plugin=%s" % protoc_gen_swagger.path)
            args.add("--swagger_out=%s:%s" % (",".join(options), output_dir))
            args.add_all(["-I%s" % include for include in includes])
            args.add(proto.path)

            actions.run(
                executable = protoc,
                inputs = inputs,
                tools = tools,
                outputs = [swagger_file],
                arguments = [args],
            )

            swagger_files.append(swagger_file)

    return swagger_files

def _proto_gen_swagger_impl(ctx):
    proto = ctx.attr.proto[ProtoInfo]
    grpc_api_configuration = ctx.file.grpc_api_configuration

    return [DefaultInfo(
        files = depset(
            _run_proto_gen_swagger(
                ctx,
                direct_proto_srcs = proto.direct_sources,
                transitive_proto_srcs = ctx.files._well_known_protos + proto.transitive_sources.to_list(),
                actions = ctx.actions,
                protoc = ctx.executable._protoc,
                protoc_gen_swagger = ctx.executable._protoc_gen_swagger,
                grpc_api_configuration = grpc_api_configuration,
                single_output = ctx.attr.single_output,
            ),
        ),
    )]

protoc_gen_swagger = rule(
    attrs = {
        "proto": attr.label(
            allow_rules = ["proto_library"],
            mandatory = True,
            providers = ["proto"],
        ),
        "grpc_api_configuration": attr.label(
            allow_single_file = True,
            mandatory = False,
        ),
        "single_output": attr.bool(
            default = False,
            mandatory = False,
        ),
        "_protoc": attr.label(
            default = "@com_google_protobuf//:protoc",
            executable = True,
            cfg = "host",
        ),
        "_well_known_protos": attr.label(
            default = "@com_google_protobuf//:well_known_protos",
            allow_files = True,
        ),
        "_protoc_gen_swagger": attr.label(
            default = Label("//protoc-gen-swagger:protoc-gen-swagger"),
            executable = True,
            cfg = "host",
        ),
    },
    implementation = _proto_gen_swagger_impl,
)
