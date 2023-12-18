install_dir?=../go/cppbridge## Path to install results (.a and .h files)

ifeq ($(shell uname -s),Darwin)
sed := gsed
else
sed := sed
endif

# All headers in entrypoint
#
# This headers combined to result header-file.
# Symbols from this files used to build runtime entrypoint.
# So if the symbol is not described in these headers, no entrypoint will be created for it.
#
# We separate .h and .hpp files here, because we need _helper.hpp file with no exported symbols,
# but with internal re-used code.
headers := $(wildcard *.h)

# Platform detected from host
#
# We only support x86_64 and aarch64 platforms wich we call amd64 and arm64 respectively.
# The amd64 and arm64 constants are found in build scripts and artifacts.
# There is no provision for changing or expanding this list.
platform := $(shell uname -m)
ifeq ($(platform),x86_64)
platform := amd64
endif
ifeq ($(platform),aarch64)
platform := arm64
endif

# Here we specifie supported flavors per platform
#
# All flavors should be valid march option.
# First flavor in list is used as minimal fallback, so it will be system requirement.
# Runtime detection flavor is placed in entrypoint.cpp.template/mk.
amd64_flavors := k8 nehalem haswell
arm64_flavors := armv8-a armv8-a+crc
flavors := $($(platform)_flavors)
generic_flavor := $(firstword $(flavors))

# Since arm64 flavors uses special characters such as + and - in the name,
# in some cases we have to escape them.
escape = echo $(1) | sed 's/[^a-zA-Z0-9]/_/g'
make_escape = $(shell $(call escape,$(1)))
amd64_escaped_flavors := $(foreach flavor,$(amd64_flavors),$(call make_escape,$(flavor)))
arm64_escaped_flavors := $(foreach flavor,$(arm64_flavors),$(call make_escape,$(flavor)))
escaped_flavors := $($(platform)_escaped_flavors)

# To distinct results built with different options we use this result_suffix
result_suffix := $(compilation_mode)
ifeq ($(asan),true)
result_suffix := $(result_suffix)_asan
endif

include entrypoint.cpp.mk
include build.mk

.PHONY: help
.DEFAULT_GOAL := help
help: ## Show this message
	@echo "Commands:"
	@grep -hE '^[a-zA-Z_\./-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-10s\033[0m %s\n", $$1, $$2}'
	@echo "Options:"
	@grep -hE '^[a-zA-Z_-]+\?=.*## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = " *(?=|##) *"}; {printf "\033[33m%-20s\033[0m %s \033[30m(default %s)\033[0m\n", $$1, $$3, $$2}'

.PHONY: all
all: result/entrypoint.h ## Generate archive with flavored code and runtime init
all: result/$(platform)_entrypoint_$(result_suffix).a

.PHONY: clean
clean: ## Remove all generated content
	@-rm -rf init build result

.PHONY: install
install: result/entrypoint.h ## Copy resulting generated files to destination $(install_dir)
install: result/$(platform)_entrypoint_$(result_suffix).a
	@install $^ $(install_dir)

result/entrypoint.h: $(headers) entrypoint.h.template ## Concat all headers in one
	@mkdir -p ${@D}
	@cat $^ > $@
