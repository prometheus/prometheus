# Got from https://gist.githubusercontent.com/oquenchil/3f88a39876af2061f8aad6cdc9d7c045/raw/ae6fe65eba5a3c8076b1a5a767ca3b9d6ddba190/cc_static_library.bzl
"""Provides a rule that outputs a monolithic static library.
   The current limitation: this rule cannot be a dependency for cc_library()/cc_test()
   due of usage of DefaultInfo() provider (as it provides the simple 'files set'
   output as ar utility output), but cc_*** requires CcInfo() provider
   (as it provides compile, linking context, etc.).

   TODO: Research the ability to mix DefaultInfo() and CcInfo() for proper usage in
   cc_***() as dependency.
   """

load("@bazel_tools//tools/cpp:toolchain_utils.bzl", "find_cpp_toolchain")

TOOLS_CPP_REPO = "@bazel_tools"

def _cc_static_library_impl(ctx):
    output_lib = ctx.actions.declare_file("{}.a".format(ctx.attr.name))
    output_flags = ctx.actions.declare_file("{}.link".format(ctx.attr.name))

    cc_toolchain = find_cpp_toolchain(ctx)

    lib_sets = []
    unique_flags = {}

    for dep in ctx.attr.deps:
        # N.B.: bazel v5 dropped almost all fields from linking_context (except linker_inputs),
        # all old fields in older version were a sum of all linker_inputs' fields.
        # If you need to adopt this rule for older Bazels, you should check attrs of
        # linking_context itself.

        if ctx.attr.debug:
            print("==== cc_static_library.bzl: ================\nDEP:\n", dep)

        current_linker_input = dep[CcInfo].linking_context.linker_inputs

        # lib_sets.append(current_linker_input.libraries)

        if not hasattr(current_linker_input, "to_list"):
            fail("The linker_input (internal variable of cc_static_library() must be a Bazel built-in depset, which has the `to_list()` method, but that method did not found!")
        
        # Convert depset to Python's list() for ease of iterating over it...
        list_of_linker_inputs = current_linker_input.to_list()
        lib_sets.append(current_linker_input)

        # and store all unique flags of all libraries..
        for linker_input in list_of_linker_inputs:

            # and user flags for linking...
            unique_flags.update({
                flag: None
                for flag in linker_input.user_link_flags
            })
    
    if ctx.attr.debug:
        print("==== cc_static_library.bzl: All lib sets: type: ", type(lib_sets), "sets: ", lib_sets)


    # libraries_to_link contains the LinkerInput's. Every LinkerInput contains the "libraries" list
    # with actual list of LibraryToLink. Every LibraryToLink contains info about library type.
    # See docs: https://bazel.build/rules/lib/builtins/LinkerInput
    #           https://bazel.build/rules/lib/builtins/LibraryToLink for reference.

    libraries_to_link = depset(transitive = lib_sets)
    link_flags = unique_flags.keys()

    libs = []

    # Collect all static libs from List[LibraryToLink].
    for lib_to_link in libraries_to_link.to_list():

        libs.extend([lib.pic_static_library for lib in lib_to_link.libraries if lib.pic_static_library])
        libs.extend([
            lib.static_library
            for lib in lib_to_link.libraries
            if lib.static_library and not lib.pic_static_library
        ])

    # generate script for AR tool.
    script_file = ctx.actions.declare_file("{}.mri".format(ctx.attr.name))
    commands = ["create {}".format(output_lib.path)]
    for lib in libs:
        commands.append("addlib {}".format(lib.path))
    commands.append("save")
    commands.append("end")
    ctx.actions.write(
        output = script_file,
        content = "\n".join(commands) + "\n",
    )

    ctx.actions.run_shell(
        command = "{} -M < {}".format(cc_toolchain.ar_executable, script_file.path),
        inputs = [script_file] + libs + cc_toolchain.all_files.to_list(),
        outputs = [output_lib],
        mnemonic = "ArMerge",
        progress_message = "Merging static library {}".format(output_lib.path),
    )
    ctx.actions.write(
        output = output_flags,
        content = "\n".join(link_flags) + "\n",
    )
    return [
        DefaultInfo(files = depset([output_flags, output_lib])),
    ]

cc_static_library = rule(
    implementation = _cc_static_library_impl,
    attrs = {
        "debug": attr.bool(doc="Prints some debug messages (for script developing/debugging)"),
        "deps": attr.label_list(),
        "_cc_toolchain": attr.label(
            default = TOOLS_CPP_REPO + "//tools/cpp:current_cc_toolchain",
        ),
    },
    toolchains = [TOOLS_CPP_REPO + "//tools/cpp:toolchain_type"],
    incompatible_use_toolchain_transition = True,
)
