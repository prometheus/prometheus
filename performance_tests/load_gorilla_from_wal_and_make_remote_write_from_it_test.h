#pragma once

#include "load_gorilla_from_wal_and_process_data.h"

struct load_gorilla_from_wal_and_make_remote_write_from_it : load_gorilla_from_wal_and_process_data {
  std::chrono::nanoseconds process_data(PromPP::WAL::Reader& wal) const final;
  void write_metrics(Metrics&) const final;

 private:
  mutable std::chrono::nanoseconds period_ = std::chrono::nanoseconds::zero();
  mutable size_t protobuf_buffer_total_size = 0;
};
