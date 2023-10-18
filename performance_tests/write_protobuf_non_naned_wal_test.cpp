#include "write_protobuf_non_naned_wal_test.h"

#include "dummy_wal.h"
#include "prometheus/remote_write.h"

#include "third_party/protozero/pbf_writer.hpp"

#include "bare_bones/gorilla.h"  // stale_nan utils

#include "log.h"

#include <map>
#include <random>
#include <vector>

using namespace PromPP;  // NOLINT

// Source id contains label values from special label "job" and "instance"
using source_id = std::pair<std::string_view, std::string_view>;
constexpr size_t SOURCE_ID_JOB = 0;
constexpr size_t SOURCE_ID_INSTANCE = 1;
constexpr int64_t TIMESTAMP_WINDOW_STEP = 30 * 1000;  // 30s

using source_uid = uint32_t;
using source_ls_pair_id_to_uid_map = std::map<source_id, source_uid>;

// Helper class for separating samples by their ID and timestamp window.
class WalMessagesTimeWindowPack {
  constexpr static inline source_uid INVALID_SOURCE_UID = std::numeric_limits<source_uid>::max();
  std::vector<source_id> source_id_map_;
  source_ls_pair_id_to_uid_map source_id_rev_map_;
  size_t buffer_size_ = 0;
  std::mt19937 device_;

  class MessageSource {
    source_uid uid_;
    std::string buffer_{};
    protozero::pbf_writer writer_;

   public:
    const auto& id() const { return uid_; }
    auto& buffer() { return buffer_; }
    const auto& buffer() const { return buffer_; }
    auto& writer() { return writer_; }
    size_t add_message(const DummyWal::Timeseries& tmsr) {
      size_t old_buf_sz = buffer_.size();
      Prometheus::RemoteWrite::write_timeseries(writer_, tmsr);
      return buffer_.size() - old_buf_sz;
    }
    explicit MessageSource(source_uid uid) : uid_(uid), buffer_{}, writer_(buffer_) {}
    MessageSource(const MessageSource&) = delete;
    MessageSource(MessageSource&&) = delete;
  };

  std::map<source_uid, MessageSource> sources_;

  source_uid create_source_id_from_ls(const DummyWal::LabelSet& ls) {
    static std::string_view empty = "";
    size_t total_used_ls = 0;
    source_id id{empty, empty};

    // search for "job" and "instance" labels and fill in
    // the source_id pair.
    for (auto label : ls) {
      if (total_used_ls == 2) {
        break;
      }
      if (std::get<0>(label) == "job") {
        std::get<SOURCE_ID_JOB>(id) = std::get<1>(label);
        ++total_used_ls;
        continue;
      }
      if (std::get<0>(label) == "instance") {
        std::get<SOURCE_ID_INSTANCE>(id) = std::get<1>(label);
        ++total_used_ls;
        continue;
      }
    }

    if (total_used_ls < 2) {
      return INVALID_SOURCE_UID;
    }

    // check source id existence
    auto iter = source_id_rev_map_.find(id);
    if (iter == source_id_rev_map_.end()) {
      size_t next_uid = source_id_map_.size();
      source_id_map_.push_back(id);
      source_id_rev_map_.insert({id, next_uid});
      return next_uid;
    }
    return iter->second;
  }

 public:
  explicit WalMessagesTimeWindowPack(size_t seed = 0) : device_(seed) {}

  const size_t buffer_size() const noexcept { return buffer_size_; }

  bool add_message(const DummyWal::Timeseries& tmsr) {
    auto cur_msg_src_uid = create_source_id_from_ls(tmsr.label_set());
    // if source+job labels are not found
    if (cur_msg_src_uid == INVALID_SOURCE_UID) {
      return false;
    }

    auto msg_source = sources_.find(cur_msg_src_uid);
    if (msg_source == sources_.end()) {
      msg_source = sources_.emplace(cur_msg_src_uid, cur_msg_src_uid).first;
    }

    buffer_size_ += msg_source->second.add_message(tmsr);

    return true;
  }

  template <typename Ostream>
  size_t flush(int64_t timestamp_window, Ostream& os) {
    size_t encoded_size = 0;
    std::vector<source_uid> all_uids;
    all_uids.reserve(sources_.size());
    // collect all uids
    for (const auto& item : sources_) {
      all_uids.push_back(item.first);
    }
    // and shuffle them
    std::ranges::shuffle(all_uids, device_);
    for (const auto& uid : all_uids) {
      encoded_size += write_message_to(sources_.at(uid), timestamp_window, os);
    }
    return encoded_size;
  }

  void clear() {
    buffer_size_ = 0;
    sources_.clear();
  }

  template <typename Ostream>
  size_t write_message_to(const MessageSource& source, int64_t timestamp_window, Ostream& out) {
    auto& cur_buffer = source.buffer();
    uint32_t size_ui32 = cur_buffer.size();
    auto id = source.id();

    out.write(reinterpret_cast<const char*>(&id), sizeof(id));
    out.write(reinterpret_cast<const char*>(&timestamp_window), sizeof(timestamp_window));

    out.write(reinterpret_cast<const char*>(&size_ui32), sizeof(size_ui32));
    out.write(cur_buffer.data(), cur_buffer.size());
    return size_ui32 + sizeof(id) + sizeof(timestamp_window) + sizeof(size_ui32);
  }
};

void write_protobuf_non_naned_wal::execute(const Config& config, Metrics& metrics) const {
  DummyWal::Timeseries tmsr;
  DummyWal dummy_wal(input_file_full_name(config));
  auto seed_str = config.get_value_of("wal_write_samples_random_seed");
  auto seed = seed_str.empty() ? 0 : std::stoi(seed_str);

  std::ofstream outfile(output_file_full_name(config), std::ios_base::binary);
  lz4_stream::ostream out(outfile);
  if (!outfile.is_open()) {
    throw std::runtime_error("failed to open file '" + output_file_full_name(config) + "'");
  }

  uint64_t encoded_size = 0;

  WalMessagesTimeWindowPack time_window_messages(seed);
  bool first = true;
  int64_t last_timestamp = 0;

  // it's ugly here, because dummy wal can have only 1 sample in timeseries
  auto start = std::chrono::steady_clock::now();
  while (dummy_wal.read_next_segment()) {
    // read all segment with whole timeseries
    while (dummy_wal.read_next(tmsr)) {
      assert(tmsr.samples().size() == 1);
      auto smpl = tmsr.samples()[0];

      // 0. If value is stalenan then continue.
      if (BareBones::Encoding::Gorilla::isstalenan(smpl.value())) {
        continue;  // skip stalenan
      }

      // 1. if first, then simply store timestamp
      if (first) {
        first = false;
        last_timestamp = smpl.timestamp();
      }

      // 2. Check that this tmsr in range of time window
      if ((smpl.timestamp() - last_timestamp) > TIMESTAMP_WINDOW_STEP) {
        // 2.1. if it is out of range, then we will flush all stored messages into a
        //      file in random order and create a new time window.
        size_t buffer_size = time_window_messages.buffer_size();
        encoded_size += time_window_messages.flush(last_timestamp, out);
        time_window_messages.clear();

        last_timestamp = smpl.timestamp();
        // log metrics
        if (dummy_wal.cnt() % 1000000 == 0) {
          auto now = std::chrono::steady_clock::now();
          log() << "Processed: " << dummy_wal.cnt()
                << " Time per sample: " << (std::chrono::duration_cast<std::chrono::nanoseconds>(now - start).count() / dummy_wal.cnt()) << " ns" << std::endl;
          log() << "Encoded size: " << ((encoded_size + buffer_size) >> 20) << "MB" << std::endl;
        }
      }

      // 3. And encode it, in any case, as pb (every message has its unique pb encoder), and continue
      time_window_messages.add_message(tmsr);
    }
    auto now = std::chrono::steady_clock::now();

    metrics << (Metric() << "protobuf_wal_add_sample_avg_duration_nanoseconds"
                         << std::chrono::duration_cast<std::chrono::nanoseconds>(now - start).count() / dummy_wal.cnt());
    metrics << (Metric() << "protobuf_wal_overall_size_megabytes" << (encoded_size >> 20));
  }
}
