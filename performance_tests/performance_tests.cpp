#include "full_load_lss_test.h"
#include "full_save_lss_test.h"
#include "load_gorilla_from_wal_and_calculate_hash_over_label_set_names_test.h"
#include "load_gorilla_from_wal_and_calculate_hash_over_label_sets_test.h"
#include "load_gorilla_from_wal_and_iterate_over_label_name_and_value_ids_test.h"
#include "load_gorilla_from_wal_and_iterate_over_label_names_and_values_test.h"
#include "load_gorilla_from_wal_and_iterate_over_label_set_ids_test.h"
#include "load_gorilla_from_wal_and_iterate_over_label_set_names_test.h"
#include "load_gorilla_from_wal_and_iterate_over_sample_label_name_ids_test.h"
#include "load_gorilla_from_wal_and_iterate_over_series_label_name_ids_test.h"
#include "load_gorilla_from_wal_and_make_remote_write_from_it_test.h"
#include "load_lss_from_wal_test.h"
#include "load_ordered_indexing_table_in_loop_test.h"
#include "load_protobuf_non_naned_wal_and_process_it_with_stale_nans.h"
#include "load_protobuf_wal_and_save_gorilla_to_sharded_wal_test.h"
#include "load_protobuf_wal_and_save_gorilla_to_wal_test.h"
#include "load_protobuf_wal_and_save_gorilla_to_wal_with_redundants_test.h"
#include "save_gorilla_to_wal_test.h"
#include "save_lss_to_wal_test.h"
#include "tests_database.h"
#include "write_protobuf_non_naned_wal_test.h"
#include "write_protobuf_wal_test.h"

#include "config.h"

#include <iostream>

static void print_help(const std::string& app_name) {
  std::cout << "Usage: " << app_name << " <command [command_args]>" << std::endl;
  std::cout << "Possible commands:" << std::endl;
  std::cout << "query <about <query_type>>" << std::endl;
  std::cout << "  where 'query_type' can be:" << std::endl;
  std::cout << "    - number_of_tests - get number of tests" << std::endl;
  std::cout << "    - test_name <test_number <some value from 1>> - get name of the test with given number" << std::endl;
  std::cout << "    - test_name <output_file_name <some file name>> <input_data_ordering <one from LS_TS, TS_LS, LS, TS, R>> - get name of the test which "
               "generates given file as output"
            << std::endl;
  std::cout << "    - input_file_name <test <some value from 1 or string>> <input_data_ordering <one from LS_TS, TS_LS, LS, TS, R>> - get input file name for "
               "given test"
            << std::endl
            << std::endl;
  std::cout << "run <target <target_id>> <path_to_test_data <some string>> <input_data_ordering <ordering type>> [branch <some string>] [commit_hash <some "
               "string>] [metrics_file_full_name <some value>] [quiet <true>]"
            << std::endl;
  std::cout << "  where 'target_id' can be any number or name of test" << std::endl;
  std::cout << "  and 'input_data_ordering' can be:" << std::endl;
  std::cout << "    - LS_TS  - input data sorted by label sets and timestamps" << std::endl;
  std::cout << "    - TS_LS  - input data sorted by timestamps and label sets" << std::endl;
  std::cout << "    - LS     - input data sorted by label sets" << std::endl;
  std::cout << "    - TS     - input data sorted by timestamps" << std::endl;
  std::cout << "    - LS_RTS - input data sorted by label sets" << std::endl;
  std::cout << "    - TS_RLS - input data sorted by timestamps" << std::endl;
  std::cout << "    - R      - input data is randomly structured" << std::endl << std::endl;
  std::cout << "help - show this text" << std::endl;
}

int main([[maybe_unused]] int argc, char* argv[]) {
  if (argc == 1) {
    print_help(argv[0]);
    return 1;
  }
  try {
    TestsDatabase test_db;
    test_db.add(std::make_unique<save_lss_to_wal>());
    test_db.add(std::make_unique<load_lss_from_wal>());
    test_db.add(std::make_unique<full_save_lss>());
    test_db.add(std::make_unique<full_load_lss>());
    test_db.add(std::make_unique<load_ordered_indexing_table_in_loop>());
    test_db.add(std::make_unique<save_gorilla_to_wal>());
    test_db.add(std::make_unique<load_gorilla_from_wal_and_iterate_over_label_set_ids>());
    test_db.add(std::make_unique<load_gorilla_from_wal_and_iterate_over_sample_label_name_ids>());
    test_db.add(std::make_unique<load_gorilla_from_wal_and_iterate_over_series_label_name_ids>());
    test_db.add(std::make_unique<load_gorilla_from_wal_and_iterate_over_label_set_names>());
    test_db.add(std::make_unique<load_gorilla_from_wal_and_iterate_over_label_name_and_value_ids>());
    test_db.add(std::make_unique<load_gorilla_from_wal_and_iterate_over_label_names_and_values>());
    test_db.add(std::make_unique<load_gorilla_from_wal_and_calculate_hash_over_label_set_names>());
    test_db.add(std::make_unique<load_gorilla_from_wal_and_calculate_hash_over_label_sets>());
    test_db.add(std::make_unique<load_gorilla_from_wal_and_make_remote_write_from_it>());
    test_db.add(std::make_unique<write_protobuf_wal>());
    test_db.add(std::make_unique<write_protobuf_non_naned_wal>());
    test_db.add(std::make_unique<load_protobuf_non_naned_wal_and_process_it_with_stale_nans>());  // depends on prev. test
    test_db.add(std::make_unique<load_protobuf_wal_and_save_gorilla_to_wal>());
    test_db.add(std::make_unique<load_protobuf_wal_and_save_gorilla_to_sharded_wal>());
    test_db.add(std::make_unique<load_protobuf_wal_and_save_gorilla_to_wal_with_redundants>());

    Config config;
    config.parameter(argv[0]);
    config.parameter("about");
    config.parameter("test");
    config.parameter("test_number");
    config.parameter("output_file_name");
    config.parameter("target");
    config.parameter("path_to_test_data");
    config.parameter("input_data_ordering");
    config.parameter("metrics_file_full_name");
    config.parameter("branch");
    config.parameter("commit_hash");
    config.parameter("quiet");
    config.load(argv, argc);

    if (config.get_value_of(argv[0]) == "query") {
      std::cout << test_db.query(config) << std::endl;
    } else if (config.get_value_of(argv[0]) == "run") {
      test_db.run(config);
    } else if (config.get_value_of(argv[0]) == "help") {
      print_help(argv[0]);
    } else {
      throw std::runtime_error("unknown command '" + config.get_value_of(argv[0]) +
                               "',"
                               "(use '" +
                               argv[0] + " help' to get reference)");
    }
  } catch (const std::exception& e) {
    std::cerr << "Error: " << e.what() << std::endl;
    return 1;
  }

  return 0;
}
