##############################################################################
# This Makefile has need for preparing input test data for performance tests #
# and generating Go header bindings                                          #
#                                                                            #
# It assumes that user put command line arguments:                           #
# - path_to_test_data                                                        #
# - test_data_file_name                                                      #
# - perf_tests_binary                                                        #
# - scale_factor                                                             #
# when running make utility                                                  #
##############################################################################

absolute_project_dir := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
relative_project_dir := $(notdir $(patsubst %/,%,$(dir $(absolute_project_dir))))
block_converter_binary := $(absolute_project_dir)/out/block_converter
block_converter_sources := $(absolute_project_dir)/tools/block_converter
sort_type := $(shell echo $(word 2, $(subst ., ,$(test_data_file_name))) | tr a-z A-Z)

test_data_file_name_prefix := $(findstring dummy_wal, $(test_data_file_name))
ifeq "$(test_data_file_name_prefix)" "dummy_wal"
$(test_data_file_name): check_if_need_to_load_from_git_lfs check_if_need_to_convert
else
.SILENT: $(test_data_file_name)
$(test_data_file_name):
	if [ ! -f $(path_to_test_data)/converted/$(test_data_file_name) ] && [ ! -f $(path_to_test_data)/$(test_data_file_name) ]; then \
		test_dependency=$$($(perf_tests_binary) query about test_name output_file_name $(test_data_file_name) input_data_ordering $(sort_type)); \
		test_dependency_input_file_name=$$($(perf_tests_binary) query about input_file_name test $$test_dependency input_data_ordering $(sort_type)); \
		$(MAKE) path_to_test_data=$(path_to_test_data) test_data_file_name=$$test_dependency_input_file_name perf_tests_binary=$(perf_tests_binary); \
		if [ -f $(path_to_test_data)/meta.json ]; then \
			$(perf_tests_binary) run target $$test_dependency path_to_test_data $(path_to_test_data)/converted input_data_ordering $(sort_type) quiet true; \
		else \
			$(perf_tests_binary) run target $$test_dependency path_to_test_data $(path_to_test_data) input_data_ordering $(sort_type) quiet true; \
		fi; \
	fi
endif

.SILENT: check_if_need_to_load_from_git_lfs
check_if_need_to_load_from_git_lfs:
	if [ -f $(path_to_test_data)/$(test_data_file_name) ] && [ $$(stat --printf="%s" "$(path_to_test_data)/$(test_data_file_name)") -le 200 ]; then \
		path_to_load_file_from_git_lfs=$(path_to_test_data); \
		file_to_load_from_git_lfs=$(test_data_file_name); \
		while [ ! "$$path_to_load_file_from_git_lfs" = "." ]; do \
			base_dir=$(basename $$path_to_load_file_from_git_lfs); \
			path_to_load_file_from_git_lfs=$$(dirname $$path_to_load_file_from_git_lfs); \
			file_to_load_from_git_lfs=$$base_dir/$$file_to_load_from_git_lfs; \
			git lfs pull -I $$file_to_load_from_git_lfs; \
			if [ -f $(path_to_test_data)/$(test_data_file_name) ] && [ $$(stat --printf="%s" "$(path_to_test_data)/$(test_data_file_name)") -gt 200 ]; then \
				break; \
			fi; \
			git lfs pull -I $(relative_project_dir)/$$file_to_load_from_git_lfs; \
	        if [ -f $(path_to_test_data)/$(test_data_file_name) ] && [ $$(stat --printf="%s" "$(path_to_test_data)/$(test_data_file_name)") -gt 200 ]; then \
				break; \
			fi; \
		done; \
  	fi

.SILENT: check_if_need_to_convert
check_if_need_to_convert: $(block_converter_binary)
	if [ -f $(path_to_test_data)/meta.json ] && [ ! -f $(path_to_test_data)/converted/$(test_data_file_name) ]; then \
  		mkdir -p $(path_to_test_data)/converted; \
	    $(block_converter_binary) --prom-block.dir=$(dir $(path_to_test_data)) --prom-block.name=$(notdir $(path_to_test_data)) --out.dir=$(path_to_test_data)/converted --sort-order=$(sort_type) --scale-factor=$(scale_factor); \
  	fi

.SILENT: $(block_converter_binary)
$(block_converter_binary): $(block_converter_sources)/go.mod $(block_converter_sources)/go.sum $(block_converter_sources)/main.go
	mkdir -p $(dir $(block_converter_binary)); \
	cd $(block_converter_sources); \
	go mod tidy; \
	go build -o $(block_converter_binary); \
	cd $(absolute_project_root)

## New part of make to replace r-scripts
include scripts/bazel.mk

all_files := $(shell git ls-files -co --exclude-standard)
vendored := $(patsubst ./%,%,$(shell find . -path '*/vendor/*'))
our_files := $(filter-out $(vendored),$(all_files))
cc_targets := $(shell bazel query 'kind("cc_.*", //...)')
cc_files := $(patsubst ./%,%,$(shell find . -type f -regex '.*\.\(h\|hpp\|cpp\)' -a -not -path './third_party/*'))
our_cc_files := $(filter $(cc_files),$(all_files))

test:
	@echo $(cc_files)

.PHONY: help
help: ## Show this message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-10s\033[0m %s\n", $$1, $$2}'

.PHONY: build-entrypoint
build-entrypoint: ## Build entry point and copy artifacts to Go directory
	$(MAKE) -C entrypoint clean install

.PHONY: ascii
ascii: $(our_files) ## Check there is no non-ascii symbols in code
	@if grep -I -H --color='auto' -P -n "[^[:ascii:]·–—]" $^; then \
		exit 1;\
	fi

.PHONY: format
format: $(our_cc_files)
	@clang-format --dry-run  --Werror $^

.PHONY: tidy
tidy: ## Check clang-tidy for all librarie code
tidy: tidy_flags := --aspects @bazel_clang_tidy//clang_tidy:clang_tidy.bzl%clang_tidy_aspect
tidy: tidy_flags += --@bazel_clang_tidy//:clang_tidy_config=//:clang_tidy_config
tidy: compilation_mode := fastbuild
tidy:
	@$(bazel_build) $(tidy_flags) --output_groups=report --platform_suffix=tidy $(cc_targets)
