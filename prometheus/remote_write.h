#pragma once

#include <string_view>

#define PROTOZERO_USE_VIEW std::string_view
#include "third_party/protozero/pbf_reader.hpp"
#include "third_party/protozero/pbf_writer.hpp"

namespace PromPP {
namespace Prometheus {
namespace RemoteWrite {

struct TimeseriesProtobufHashdexRecord {
  size_t labelset_hashval;
  std::string_view timeseries_protobuf_message;

  inline __attribute__((always_inline)) explicit TimeseriesProtobufHashdexRecord(size_t lshv, std::string_view& tpm) noexcept
      : labelset_hashval(lshv), timeseries_protobuf_message(tpm) {}
};

template <class ProtobufWriter, class Sample>
inline __attribute__((always_inline)) void write_sample(ProtobufWriter& pb, const Sample& sample) {
  protozero::pbf_writer pb_sample(pb, 2);
  pb_sample.add_double(1, sample.value());
  pb_sample.add_int64(2, sample.timestamp());
}

template <class ProtobufWriter>
inline __attribute__((always_inline)) void write_label(ProtobufWriter& pb, const std::string_view& label_name, const std::string_view& label_value) {
  protozero::pbf_writer pb_label(pb, 1);
  pb_label.add_string(1, label_name);
  pb_label.add_string(2, label_value);
}

template <class ProtobufWriter, class LabelSet>
inline __attribute__((always_inline)) void write_label_set(ProtobufWriter& pb, const LabelSet& label_set) {
  for (const auto& [label_name, label_value] : label_set) {
    write_label(pb, label_name, label_value);
  }
}

template <class ProtobufWriter, class Timeseries>
inline __attribute__((always_inline)) void write_timeseries(ProtobufWriter& pb, const Timeseries& timeseries) {
  protozero::pbf_writer pb_timeseries(pb, 1);

  write_label_set(pb_timeseries, timeseries.label_set());

  for (const auto& sample : timeseries.samples()) {
    write_sample(pb_timeseries, sample);
  }
}

template <class ProtobufReader, class Sample>
inline __attribute__((always_inline)) void read_sample(ProtobufReader& pb_sample, Sample& sample) {
  uint8_t parsed = 0;

  while (pb_sample.next()) {
    switch (pb_sample.tag()) {
      case 1:  // value
        sample.value() = pb_sample.get_double();
        parsed |= 0b01;
        break;
      case 2:  // timestamp
        sample.timestamp() = pb_sample.get_int64();
        parsed |= 0b010;
        break;
      default:
        pb_sample.skip();
    }
  }

  if (__builtin_expect(parsed != 0b11, false)) {
    throw std::runtime_error("AOD3: Meaningful message supposed to be here!");
  }
}

template <class ProtobufReader, class Label>
inline __attribute__((always_inline)) void read_label(ProtobufReader& pb_label, Label& label) {
  uint8_t parsed = 0;

  while (pb_label.next()) {
    switch (pb_label.tag()) {
      case 1:  // label name
        std::get<0>(label) = pb_label.get_view();
        parsed |= 0b01;
        break;
      case 2:  // label value
        std::get<1>(label) = pb_label.get_view();
        parsed |= 0b10;
        break;
      default:
        pb_label.skip();
    }
  }

  if (__builtin_expect(parsed != 0b11, false)) {
    throw std::runtime_error("AOC3: Meaningful message supposed to be here!");
  }
}

template <class ProtobufReader, class LabelSet>
inline __attribute__((always_inline)) void read_only_label_set(ProtobufReader& pb_timeseries, LabelSet& label_set) {
  while (pb_timeseries.next(1)) {
    auto pb_label = pb_timeseries.get_message();
    typename LabelSet::label_type label;
    read_label(pb_label, label);
    label_set.add(label);
  }

  if (__builtin_expect(!label_set.size(), false)) {
    throw std::runtime_error("AOB3: Meaningful message supposed to be here!");
  }
}

template <class ProtobufReader, class Timeseries>
inline __attribute__((always_inline)) void read_timeseries(ProtobufReader&& pb_timeseries, Timeseries& timeseries) {
  while (pb_timeseries.next()) {
    switch (pb_timeseries.tag()) {
      case 1: {  // label
        auto pb_label = pb_timeseries.get_message();
        typename Timeseries::label_set_type::label_type label;
        read_label(pb_label, label);
        timeseries.label_set().add(label);
      } break;

      case 2: {  // sample
        auto& samples = timeseries.samples();
        samples.resize(samples.size() + 1);
        auto pb_sample = pb_timeseries.get_message();
        try {
          read_sample(pb_sample, samples.back());
        } catch (...) {
          samples.resize(samples.size() - 1);
          throw;
        }
      } break;

      default:
        pb_timeseries.skip();
    }
  }

  if (__builtin_expect(!timeseries.label_set().size() || !timeseries.samples().size(), false)) {
    throw std::runtime_error("AOA3: Meaningful message supposed to be here!");
  }
}

template <class Timeseries, class ProtobufReader, class Callback>
  requires std::is_invocable<Callback, const Timeseries&>::value
__attribute__((flatten)) void read_many_timeseries(ProtobufReader& pb, Callback func) {
  Timeseries timeseries;

  try {
    while (pb.next(1)) {
      auto pb_timeseries = pb.get_message();
      read_timeseries(pb_timeseries, timeseries);
      func(timeseries);
      timeseries.clear();
    }
  } catch (protozero::exception& e) {
    throw std::runtime_error("AOE1: Meaningful message supposed to be here!");
  }
}

template <class ProtobufReader, class Timeseries>
inline __attribute__((always_inline)) void read_timeseries_without_samples(ProtobufReader&& pb_timeseries, Timeseries& timeseries) {
  while (pb_timeseries.next(1)) {
    auto pb_label = pb_timeseries.get_message();
    typename Timeseries::label_set_type::label_type label;
    read_label(pb_label, label);
    timeseries.label_set().add(label);
  }

  if (__builtin_expect(!timeseries.label_set().size(), false)) {
    throw std::runtime_error("read_timeseries_without_samples: Meaningful message supposed to be here!");
  }
}

template <class Timeseries, class Hashdex, class ProtobufReader>
  requires std::is_same<typename Hashdex::value_type, TimeseriesProtobufHashdexRecord>::value
__attribute__((flatten)) void read_many_timeseries_in_hashdex(ProtobufReader& pb, Hashdex& hdx) {
  Timeseries timeseries;

  try {
    while (pb.next(1)) {
      auto pb_view = pb.get_view();
      read_timeseries_without_samples(protozero::pbf_reader{pb_view}, timeseries);
      hdx.emplace_back(hash_value(timeseries.label_set()), pb_view);
      timeseries.clear();
    }
  } catch (protozero::exception& e) {
    throw std::runtime_error("read_many_timeseries_with_sharding: Meaningful message supposed to be here!");
  }
}
}  // namespace RemoteWrite
}  // namespace Prometheus
}  // namespace PromPP
