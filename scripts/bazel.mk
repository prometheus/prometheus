# This is includ file with 

compilation_mode?=opt## Bazel compilation mode opt or dbg
asan?=false## Build with address sanitizer
bazel_verbose?=false## More verbose output

# Bazel workspace directory relative to this file
bazel_workspace_root := $(abspath $(dir $(abspath $(lastword $(MAKEFILE_LIST))))..)

bazel := bazel

# On host system bazel run inside docker
#
# For general purpose using alpine (musl runtime bindings) as production build.
# But musl doesn't support sanitizers and we use based on ubuntu (glibc) environment.
ifeq (,$(wildcard /.dockerenv))
ifeq ($(asan),true)
bazel := ./scripts/run_in_ubuntu.sh $(bazel)
else
bazel := ./scripts/run_in_alpine.sh $(bazel)
endif
endif

bazel_flags = --compilation_mode=$(compilation_mode)
ifeq ($(asan),true)
bazel_flags += --asan --strip=never --platform_suffix=asan
endif
ifeq ($(bazel_verbose),true)
bazel_flags += --subcommands --verbose_failures --sandbox_debug
endif

# bazel_in_root is a shortcut to run command in project root
bazel_in_root := cd $(bazel_workspace_root)

# bazel_build exposed to bazel build command prefix
#
# Use this string in manner
# ```
# $(bazel_in_root);\
# 	$(bazel_build) -- //:my-target
# ```
#
# In cases if you need to specify concrete march option (default is native),
# use bazel_build_march
bazel_build = $(bazel) build $(bazel_flags)

# bazel_build_march is a function to add --march flag to standard bazel_build
#
# Use this function in manner
# ```
# $(bazel_in_root);\
# 	$(call bazel_build_march,nehalem) -- //:my_target
# ```
bazel_build_march = $(bazel_build) --march=$(1) --platform_suffix=$$(echo $(1) | sed 's/[^a-zA-Z0-9]/_/g')
