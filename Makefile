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

BUILD_DIR := bazel-bin/

# arch default value is a intentionally incorrect as bash doesn't like empty arguments.
# In the default case, the go_bindings_arch (if not defined) would be initialized with
# buildsystem arch name converted from uname.
ARCH ?= uname-defined

go_bindings_arch ?= $(shell ./r --arch-from-arg $(ARCH) || ./r --arch-from-uname)
go_in_wal_h_file := wal/wal_c_api.h
wal_c_api_static_filename := $(go_bindings_arch)_wal_c_api_static.a
go_out_filename := $(go_bindings_arch)_wal_c_api

go_out_bindings_dir := go/common/internal/

uid := $(shell id -u)
gid := $(shell id -g)
platform := $(shell arch)

wal_bindings: $(go_in_wal_h_file)
	./r --static-c-api --arch $(go_bindings_arch)
	mkdir -p $(go_out_bindings_dir)
	cp -f $(BUILD_DIR)$(wal_c_api_static_filename) $(go_out_bindings_dir)$(go_out_filename).a
	chown $(uid):$(gid) $(go_out_bindings_dir)$(go_out_filename).a


with_docker wal_bindings_with_docker: $(go_in_wal_h_file)
	docker run --pull always --rm -v.:/src --workdir /src registry.flant.com/okmeter/pp:gcc-tools-$(platform) bash -c "\
	./r --static-c-api --arch $(go_bindings_arch) && \
	mkdir -p $(go_out_bindings_dir) && \
	cp -f $(BUILD_DIR)$(wal_c_api_static_filename) $(go_out_bindings_dir)$(go_out_filename).a && \
	chown $(uid):$(gid) $(go_out_bindings_dir)$(go_out_filename).a"


asan wal_bindings_asan: $(go_in_wal_h_file)
	./r --static-c-api --asan --arch $(go_bindings_arch)
	mkdir -p $(go_out_bindings_dir)
	cp -f $(BUILD_DIR)$(wal_c_api_static_filename) $(go_out_bindings_dir)$(go_out_filename)_asan.a


dbg wal_bindings_dbg: $(go_in_wal_h_file)
	./r --static-c-api --dbg --arch $(go_bindings_arch)
	mkdir -p $(go_out_bindings_dir)
	cp -f $(BUILD_DIR)$(wal_c_api_static_filename) $(go_out_bindings_dir)$(go_out_filename)_dbg.a


dbg_asan wal_bindings_dbg_asan: $(go_in_wal_h_file)
	./r --static-c-api --dbg --asan --arch $(go_bindings_arch)
	mkdir -p $(go_out_bindings_dir)
	cp -f $(BUILD_DIR)$(wal_c_api_static_filename) $(go_out_bindings_dir)$(go_out_filename)_dbg_asan.a


clean:
	rm -f $(go_out_bindings_dir)$(go_out_filename).a
	rm -f $(go_out_bindings_dir)$(go_out_filename)_asan.a
	rm -f $(go_out_bindings_dir)$(go_out_filename)_dbg.a
	rm -f $(go_out_bindings_dir)$(go_out_filename)_dbg_asan.a

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
