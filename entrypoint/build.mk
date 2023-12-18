# This line should be placed before any include
build_dir_absolute_path := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))build

include ../scripts/bazel.mk

archives := $(patsubst %, build/$(platform)_%_entrypoint_aio.a, $(escaped_flavors))
prefixed_archives := $(patsubst %.a, %_prefixed.a, $(archives))

result/$(platform)_entrypoint_$(result_suffix).a: build/entrypoint.mri
	@mkdir -p ${@D}
	@ar -M < $<

.PRECIOUS: build/entrypoint.mri
build/entrypoint.mri: build/entrypoint_init_aio.a
build/entrypoint.mri: $(prefixed_archives)
	@printf 'create %s\n' "result/$(platform)_entrypoint_$(result_suffix).a" > $@
	@for i in $^; do\
		printf 'addlib %s\n' "$$i";\
	done >> $@
	@printf 'save\nend\n' >> $@

build/entrypoint_init_aio.a: init/entrypoint.cpp
	@mkdir -p ${@D}
	@$(bazel_in_root);\
		$(call bazel_build_march,$(generic_flavor)) -- //:entrypoint_init_aio
	@cp -f ../bazel-bin/${@F} $@

# Build flavoured prefixed_archives with prefixed symbols
.PRECIOUS: $(prefixed_archives)
$(prefixed_archives): build/%_entrypoint_aio_prefixed.a: build/%.pairs | build/%_entrypoint_aio.a
	@objcopy --redefine-syms=$< $| $@

.INTERMEDIATE: build/%.pairs
build/%.pairs: build/%.symbols
	@cat $< | grep ' [tTdDrRbB] ' | cut -d' ' -f3 | sort -u | awk '{ print $$0 " $(call make_escape,$*)_" $$0 }' > $@

.INTERMEDIATE: build/%.symbols
build/%.symbols: build/%_entrypoint_aio.a
	@nm --defined-only $< > $@


# We build all archives in bash loop because files contains escaped flavor in name but --march flag shouldn't be escaped
.PRECIOUS: $(archives)
$(archives): always
	@mkdir -p build
	@$(bazel_in_root);\
		for i in $(flavors); do\
			$(call bazel_build_march,$$i) -- //:entrypoint_aio;\
			cp -f bazel-bin/entrypoint_aio.a $(build_dir_absolute_path)/$(platform)_$$($(call escape,$$i))_entrypoint_aio.a;\
		done

# This is just a hack to always run some steps
.PHONY: always
always: