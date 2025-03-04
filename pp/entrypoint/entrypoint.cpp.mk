amd64_headers := $(patsubst %.h, init/%_amd64.h, $(headers))
arm64_headers := $(patsubst %.h, init/%_arm64.h, $(headers))
amd64_bindings := $(foreach flavor,$(amd64_escaped_flavors),init/entrypoint.amd64_$(flavor)_bindings)
arm64_bindings := $(foreach flavor,$(arm64_escaped_flavors),init/entrypoint.arm64_$(flavor)_bindings)

.PRECIOUS: init/entrypoint.cpp
init/entrypoint.cpp: ## Built runtime entrypoint with flavor determine init function
init/entrypoint.cpp: init/entrypoint.amd64_includes | $(amd64_headers)
init/entrypoint.cpp: init/entrypoint.arm64_includes | $(arm64_headers)
init/entrypoint.cpp: init/entrypoint.function_pointers
init/entrypoint.cpp: $(amd64_bindings)
init/entrypoint.cpp: $(arm64_bindings)
init/entrypoint.cpp: entrypoint.cpp.template
	@while IFS="" read -r p || [ -n "$$p" ]; do \
		case $$p in \
		//#*) cat init/$$(echo "$$p" | $(sed) 's@^//#@@');; \
		*) printf '%s\n' "$$p";; \
		esac; \
	done < $< > $@
	@rm $(filter init/%, $^)

define prefix_functions
	echo "// This file is generated. DO NOT EDIT." > $@
	echo "" >> $@
	cat $^ | $(sed) '/^void /{h; $(patsubst %, g; s//void $(1)_%_/p;, $(2)) d}' >> $@
endef

.PRECIOUS: $(amd64_headers)
$(amd64_headers): init/%_amd64.h: %.h
	@mkdir -p ${@D}
	@$(call prefix_functions, amd64, $(amd64_escaped_flavors))

.PRECIOUS: $(arm64_headers)
$(arm64_headers): init/%_arm64.h: %.h
	@mkdir -p ${@D}
	@$(call prefix_functions, arm64, $(arm64_escaped_flavors))

.INTERMEDIATE: init/entrypoint.%_includes
init/entrypoint.%_includes: $(headers)
	@mkdir -p ${@D}
	@for i in $(patsubst %.h, %, $^); do\
		printf '#include "%s_%s.h"\n' "$$i" "$*";\
	done > $@

.INTERMEDIATE: init/entrypoint.function_pointers
init/entrypoint.function_pointers: init/all.symbols
	@while IFS="" read -r p || [ -n "$$p" ]; do\
		fn_name=$$(echo "$$p" | $(sed) 's/^void //;s/(.*);//');\
		arg_def=$$(echo "$$p" | $(sed) 's/^.*(\(.*\));/\1/');\
		arg_names=$$(echo $$arg_def | $(sed) 's/void\* //g');\
		printf 'void (*%s_ptr)(%s);\n' "$$fn_name" "$$arg_def";\
		printf 'extern "C" void %s(%s) {\n' "$$fn_name" "$$arg_def";\
		printf '  (*%s_ptr)(%s);\n' "$$fn_name" "$$arg_names";\
		printf '}\n\n';\
	done < $^ > $@

.INTERMEDIATE: init/entrypoint.%_bindings
$(amd64_bindings) $(arm64_bindings): init/entrypoint.%_bindings: init/all.symbols
	@while IFS="" read -r p || [ -n "$$p" ]; do\
		fn_name=$$(echo "$$p" | $(sed) 's/^void //;s/(.*);//');\
		printf '    %s_ptr = &$*_%s;\n' "$$fn_name" "$$fn_name";\
	done < $^ > $@

.INTERMEDIATE: init/all.symbols
init/all.symbols: $(headers)
	@cat $^ | grep '^void ' > $@
