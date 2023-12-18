#pragma once

#include <cstdint>
#include <string_view>

#include "bare_bones/exception.h"
#include "bare_bones/vector.h"
#include "primitives/primitives.h"
#include "prometheus/remote_write.h"

#include "third_party/protozero/pbf_reader.hpp"

namespace PromPP::WAL {
class Hashdex {
 public:
  // Determines the sharded en/decoding constraints for label_sets and protobufs.
  // Use it to limit the hashdex's memory consumption for sharding protobufs.
  struct label_set_limits {
    uint32_t max_label_name_length;
    uint32_t max_label_value_length;
    uint32_t max_label_names_per_timeseries;
    size_t max_timeseries_count;
    size_t max_pb_size_in_bytes;
  };

  class Item {
    size_t hash_;
    std::string_view data_;

   public:
    inline __attribute__((always_inline)) explicit Item(size_t hash, std::string_view data) : hash_(hash), data_(data) {}
    inline __attribute__((always_inline)) size_t hash() const { return hash_; }

    template <class Timeseries>
    inline __attribute__((always_inline)) void read(Timeseries& timeseries) const {
      Prometheus::RemoteWrite::read_timeseries(protozero::pbf_reader(data_), timeseries);
    }
  };

 private:
  BareBones::Vector<Item> items_;
  std::string_view replica_;
  std::string_view cluster_;
  const label_set_limits limits_{};  // no limits on default.

 public:
  using iterator_category = BareBones::Vector<const Item>::iterator_category;
  using value_type = const Item;
  using const_iterator = BareBones::Vector<const Item>::const_iterator;

  inline __attribute__((always_inline)) Hashdex() noexcept {}
  inline __attribute__((always_inline)) Hashdex(const label_set_limits& limits) noexcept : limits_(limits) {}
  inline __attribute__((always_inline)) ~Hashdex(){};

  constexpr const std::string_view replica() const noexcept { return replica_; }
  constexpr const std::string_view cluster() const noexcept { return cluster_; }
  constexpr const label_set_limits& limits() const noexcept { return limits_; }

  // presharding - from protobuf make presharding slice with hash end proto.
  inline __attribute__((always_inline)) void presharding(const char* proto_data, size_t proto_len) {
    if (limits_.max_pb_size_in_bytes && proto_len > limits_.max_pb_size_in_bytes) {
      throw BareBones::Exception(0x1d979f3023b86c48, "Protobuf message's size (%zd) exceeds the maximum protobuf message size (%zd)", proto_len,
                                 limits_.max_pb_size_in_bytes);
    }

    size_t current_timeseries_n = 0;
    Prometheus::RemoteWrite::PbLabelSetMemoryLimits pb_limits = {
        limits_.max_label_name_length,
        limits_.max_label_value_length,
        limits_.max_label_names_per_timeseries,
        limits_.max_timeseries_count,
    };
    Primitives::LabelViewSet label_set;
    protozero::pbf_reader pb(std::string_view{proto_data, proto_len});
    bool first = true;
    try {
      while (pb.next(1)) {
        if (pb_limits.max_timeseries_count && current_timeseries_n >= pb_limits.max_timeseries_count) {
          throw BareBones::Exception(0xdedb5b24d946cc4d, "Max Timeseries count limit exceeded");
          break;
        }
        auto pb_view = pb.get_view();
        Prometheus::RemoteWrite::read_timeseries_label_set(protozero::pbf_reader{pb_view}, label_set, pb_limits);
        items_.emplace_back(hash_value(label_set), pb_view);
        if (first) {
          first = false;
          for (const auto& [name, value] : label_set) {
            if (name == "cluster") {
              cluster_ = value;
            }
            if (name == "__replica__") {
              replica_ = value;
            }
          }
        }
        label_set.clear();
        current_timeseries_n++;
      }
    } catch (protozero::exception& e) {
      throw BareBones::Exception(0xbe40bda82f01b869, "Protobuf parsing timeseries exception: %s", e.what());
    }
  };

  inline __attribute__((always_inline)) const_iterator begin() const noexcept { return std::begin(items_); }
  inline __attribute__((always_inline)) const_iterator end() const noexcept { return std::end(items_); }
};
}  // namespace PromPP::WAL
