#pragma once

#include <scope_exit.h>
#include <chrono>
#include <cstdlib>
#include <iostream>
#include <iterator>
#include <limits>
#include <stdexcept>

#include "third_party/uuid.h"

#include "bare_bones/crc32.h"
#include "bare_bones/encoding.h"
#include "bare_bones/exception.h"
#include "bare_bones/gorilla.h"
#include "bare_bones/snug_composite.h"
#include "bare_bones/sparse_vector.h"
#include "bare_bones/vector.h"

#include "primitives/primitives.h"
#include "primitives/snug_composites.h"

namespace PromPP::WAL {

template <class LabelSetsTable = Primitives::SnugComposites::LabelSet::EncodingBimap>
class BasicEncoder {
 public:
  class Buffer {
    BareBones::SparseVector<Primitives::Sample> singular_;
    BareBones::SparseVector<BareBones::Vector<Primitives::Sample>> plural_;
    uint32_t samples_count_ = 0;
    uint32_t series_count_ = 0;
    Primitives::Timestamp earliest_sample_ = std::numeric_limits<Primitives::Timestamp>::max();
    Primitives::Timestamp latest_sample_ = 0;
    int64_t first_sample_added_at_tsns_ = 0;

   public:
    inline __attribute__((always_inline)) uint32_t samples_count() const { return samples_count_; }

    inline __attribute__((always_inline)) uint32_t series_count() const { return series_count_; }

    inline __attribute__((always_inline)) Primitives::Timestamp earliest_sample() const { return earliest_sample_; }

    inline __attribute__((always_inline)) Primitives::Timestamp latest_sample() const { return latest_sample_; }

    inline __attribute__((always_inline)) int64_t first_sample_added_at_ts_ns() const { return first_sample_added_at_tsns_; }

    inline __attribute__((always_inline)) void clear() {
      singular_.clear();
      plural_.clear();

      samples_count_ = 0;
      series_count_ = 0;

      earliest_sample_ = std::numeric_limits<Primitives::Timestamp>::max();
      latest_sample_ = 0;
      first_sample_added_at_tsns_ = 0;
    }

    template <class T>
    inline __attribute__((always_inline)) void add(Primitives::LabelSetID ls_id, const T& smpl) {
      // TODO What to do with non unique timestamps?

      if (first_sample_added_at_tsns_ == 0) {
        const auto now = std::chrono::system_clock::now();
        first_sample_added_at_tsns_ = std::chrono::duration_cast<std::chrono::nanoseconds>(now.time_since_epoch()).count();
      }

      ++samples_count_;

      earliest_sample_ = std::min(smpl.timestamp(), earliest_sample_);
      latest_sample_ = std::max(smpl.timestamp(), earliest_sample_);

      if (ls_id >= singular_.size()) {
        singular_.resize(ls_id + 1 + 512);
      }

      if (__builtin_expect(plural_.count(ls_id), false)) {
        auto& vec = plural_[ls_id];

        if (__builtin_expect(vec.back().timestamp() < smpl.timestamp(), true)) {
          vec.push_back(Primitives::Sample(smpl));
        } else {
          vec.insert(std::lower_bound(vec.begin(), vec.end(), smpl, [](const Primitives::Sample& a, const T& b) { return a.timestamp() < b.timestamp(); }),
                     Primitives::Sample(smpl));
        }
      } else if (__builtin_expect(singular_.count(ls_id), false)) {
        plural_.resize(ls_id + 1 + 512);

        const auto& first_smpl = singular_[ls_id];
        auto& vec = plural_[ls_id];

        if (__builtin_expect(first_smpl.timestamp() < smpl.timestamp(), true)) {
          vec.push_back(first_smpl);
          vec.push_back(Primitives::Sample(smpl));
        } else {
          vec.push_back(Primitives::Sample(smpl));
          vec.push_back(first_smpl);
        }
      } else {
        ++series_count_;
        singular_[ls_id] = smpl;
      }
    }

    template <class Callback>
      requires std::is_invocable_v<Callback, Primitives::LabelSetID, Primitives::Timestamp, Primitives::Sample::value_type>
    __attribute__((flatten)) void for_each(Callback func) const {
      if (__builtin_expect(plural_.empty(), true)) {
        for (const auto& [ls_id, s] : singular_) {
          func(ls_id, s.timestamp(), s.value());
        }
      } else {
        for (const auto& [ls_id, s] : singular_) {
          if (__builtin_expect(plural_.count(ls_id), true)) {
            for (const auto& s : plural_[ls_id]) {
              func(ls_id, s.timestamp(), s.value());
            }
          } else {
            func(ls_id, s.timestamp(), s.value());
          }
        }
      }
    }
  };

  struct __attribute__((__packed__)) EncoderWithID {
    BareBones::Encoding::Gorilla::StreamEncoder encoder;
    Primitives::LabelSetID id;
  };

  struct Redundant {
    uint32_t segment;
    uint32_t encoders_count;
    typename LabelSetsTable::checkpoint_type label_sets_checkpoint;
    BareBones::Vector<EncoderWithID> encoders;

    inline __attribute__((always_inline))
    Redundant(uint32_t _segment, typename LabelSetsTable::checkpoint_type _label_sets_checkpoint, uint32_t _encoders_count)
        : segment(_segment), encoders_count(_encoders_count), label_sets_checkpoint(_label_sets_checkpoint) {}
  };

 private:
  LabelSetsTable label_sets_;
  typename LabelSetsTable::checkpoint_type label_sets_checkpoint_;
  Buffer buffer_;
  BareBones::Vector<BareBones::Encoding::Gorilla::StreamEncoder> gorilla_;

  const uuids::uuid uuid_;
  uint32_t next_encoded_segment_ = 0;

  Primitives::Timestamp ts_base_ = std::numeric_limits<Primitives::Timestamp>::max();

  // FIXME: why is it not same size as the similar field in writer..
  uint32_t samples_ = 0;
  uint64_t label_sets_bytes_ = 0;
  uint64_t ls_id_bytes_ = 0;
  uint64_t ts_bytes_ = 0;
  uint64_t v_bytes_ = 0;
  uint64_t metadata_bytes_ = 0;
  uint16_t shard_id_ = 0;
  uint8_t pow_two_of_total_shards_ = 0;

  // TODO: refactor this function to avoid the new-allocated Redundant*
  // in favour of std::unique_ptr<> or another.
  template <class OutputStream>
  Redundant* encode_segment(OutputStream& out) {
    BareBones::BitSequence gorilla_ts_bitseq, gorilla_v_bitseq;
    BareBones::EncodedSequence<BareBones::Encoding::DeltaRLE<>> ls_id_delta_rle_seq;
    BareBones::EncodedSequence<BareBones::Encoding::DeltaZigZagRLE<>> ts_delta_rle_seq;

    BareBones::CRC32 ls_id_crc, ts_crc, v_crc;

    bool ts_delta_rle_is_worth_trying = buffer_.series_count() * 0.1 > (buffer_.samples_count() - buffer_.series_count());

    ts_base_ = std::min(ts_base_, buffer_.earliest_sample());

    // delta_rle requires delta to fit in int32
    if (buffer_.latest_sample() > std::numeric_limits<int32_t>::max() - ts_base_)
      ts_delta_rle_is_worth_trying = false;

    // gorilla requires delta to fit in int64
    if (buffer_.latest_sample() > std::numeric_limits<int64_t>::max() - ts_base_) {
      throw BareBones::Exception(0x546e143d302c4860, "The latest segment's sample timestamp (%zd) is greater than max_int64 for timestamp(%zd)",
                                 buffer_.latest_sample(), (std::numeric_limits<int64_t>::max() - ts_base_));
    }

    gorilla_.resize(label_sets_.size());

    Redundant* redundant = new Redundant(next_encoded_segment_, label_sets_checkpoint_, label_sets_.size());
    Primitives::LabelSetID last_id = std::numeric_limits<Primitives::LabelSetID>::max();
    if (ts_delta_rle_is_worth_trying) {
      buffer_.for_each([&](Primitives::LabelSetID ls_id, Primitives::Timestamp ts, Primitives::Sample::value_type v) {
        assert(ls_id >= last_id);
        if (!last_id || last_id != ls_id) {
          redundant->encoders.emplace_back(gorilla_[ls_id], ls_id);
        }
        gorilla_[ls_id].encode(ts - ts_base_, v, gorilla_ts_bitseq, gorilla_v_bitseq);

        ls_id_delta_rle_seq.push_back(ls_id);
        ts_delta_rle_seq.push_back(ts - ts_base_);

        ls_id_crc << ls_id;
        ts_crc << ts;
        v_crc << v;
      });
    } else {
      buffer_.for_each([&](Primitives::LabelSetID ls_id, Primitives::Timestamp ts, Primitives::Sample::value_type v) {
        if (!last_id || last_id != ls_id) {
          last_id = ls_id;
          redundant->encoders.emplace_back(gorilla_[ls_id], ls_id);
        }
        gorilla_[ls_id].encode(ts - ts_base_, v, gorilla_ts_bitseq, gorilla_v_bitseq);

        ls_id_delta_rle_seq.push_back(ls_id);

        ls_id_crc << ls_id;
        ts_crc << ts;
        v_crc << v;
      });
    }

    auto original_exceptions = out.exceptions();
    auto sg1 = std::experimental::scope_exit([&]() { out.exceptions(original_exceptions); });
    out.exceptions(std::ifstream::failbit | std::ifstream::badbit);

    // write version
    out.put(1);
    ++metadata_bytes_;

    // write uuid
    out.write(reinterpret_cast<const char*>(uuid_.as_bytes().data()), 16);
    metadata_bytes_ += 16;

    // write shard ID
    out.write(reinterpret_cast<const char*>(&shard_id_), 2);
    metadata_bytes_ += 2;

    // and pow of two of total shards..
    out.write(reinterpret_cast<const char*>(&pow_two_of_total_shards_), 1);
    metadata_bytes_ += 1;

    // write segment number
    out.write(reinterpret_cast<const char*>(&next_encoded_segment_), sizeof(next_encoded_segment_));
    metadata_bytes_ += sizeof(next_encoded_segment_);

    // write open-close timestamps
    const int64_t created_at_tsns = buffer_.first_sample_added_at_ts_ns();
    const auto now = std::chrono::system_clock::now();
    const int64_t encoded_at_tsns = std::chrono::duration_cast<std::chrono::nanoseconds>(now.time_since_epoch()).count();
    out.write(reinterpret_cast<const char*>(&created_at_tsns), sizeof(created_at_tsns));
    out.write(reinterpret_cast<const char*>(&encoded_at_tsns), sizeof(encoded_at_tsns));
    metadata_bytes_ += 16;

    // write new label sets
    label_sets_checkpoint_ = label_sets_.checkpoint();
    label_sets_bytes_ += label_sets_checkpoint_.save_size(&redundant->label_sets_checkpoint);
    out << (label_sets_checkpoint_ - redundant->label_sets_checkpoint);

    // write ls ids
    ls_id_bytes_ += ls_id_delta_rle_seq.save_size() + ls_id_crc.save_size();
    out << ls_id_delta_rle_seq << ls_id_crc;

    // write ts base
    out.write(reinterpret_cast<const char*>(&ts_base_), sizeof(ts_base_));
    ts_bytes_ += sizeof(ts_base_);

    // write ts
    if (ts_delta_rle_is_worth_trying && ts_delta_rle_seq.save_size() < gorilla_ts_bitseq.save_size()) {
      out.put(0);

      ts_bytes_ += ts_delta_rle_seq.save_size() + ts_crc.save_size();
      out << ts_delta_rle_seq << ts_crc;
    } else {
      out.put(1);

      ts_bytes_ += gorilla_ts_bitseq.save_size() + ts_crc.save_size();
      out << gorilla_ts_bitseq << ts_crc;
    }

    // write values
    v_bytes_ += gorilla_v_bitseq.save_size() + v_crc.save_size();
    out << gorilla_v_bitseq << v_crc;

    samples_ += buffer_.samples_count();
    buffer_.clear();
    ++next_encoded_segment_;
    return redundant;
  }

  static auto generate_uuid() {
    static uuids::uuid_random_generator gen = []() {
      std::random_device rd;
      auto seed_data = std::array<int, std::mt19937::state_size>{};
      std::generate(std::begin(seed_data), std::end(seed_data), std::ref(rd));
      std::seed_seq seq(std::begin(seed_data), std::end(seed_data));
      static std::mt19937 generator(seq);
      return uuids::uuid_random_generator(generator);
    }();

    return gen();
  }

 public:
  explicit BasicEncoder(uint16_t shard_id = 0, uint8_t pow_two_of_total_shards = 0)
      : label_sets_checkpoint_(label_sets_.checkpoint()), uuid_(generate_uuid()), shard_id_(shard_id), pow_two_of_total_shards_(pow_two_of_total_shards) {}

  inline __attribute__((always_inline)) const LabelSetsTable& label_sets() const { return label_sets_; }

  inline __attribute__((always_inline)) const Buffer& buffer() const { return buffer_; }

  inline __attribute__((always_inline)) uint16_t shard_id() const noexcept { return shard_id_; }

  inline __attribute__((always_inline)) uint8_t pow_two_of_total_shards() const noexcept { return pow_two_of_total_shards_; }

  inline __attribute__((always_inline)) uint64_t samples() const { return samples_; }

  inline __attribute__((always_inline)) uint32_t series() const { return label_sets_.size(); }

  inline __attribute__((always_inline)) uint64_t metadata_bytes() const { return metadata_bytes_; }

  inline __attribute__((always_inline)) uint64_t label_sets_bytes() const { return label_sets_bytes_; }

  inline __attribute__((always_inline)) uint64_t ls_id_bytes() const { return ls_id_bytes_; }

  inline __attribute__((always_inline)) uint64_t timestamps_bytes() const { return ts_bytes_; }

  inline __attribute__((always_inline)) uint64_t values_bytes() const { return v_bytes_; }

  inline __attribute__((always_inline)) uint64_t total_bytes() const { return metadata_bytes_ + label_sets_bytes_ + ls_id_bytes_ + ts_bytes_ + v_bytes_; }

  template <typename T>
  inline __attribute__((always_inline)) void add(const T& tmsr) {
    Primitives::LabelSetID ls_id = label_sets_.find_or_emplace(tmsr.label_set());
    for (const auto& smpl : tmsr.samples()) {
      buffer_.add(ls_id, smpl);
    }
  }

  template <typename T>
  inline __attribute__((always_inline)) void add(const T& tmsr, size_t hashval) {
    Primitives::LabelSetID ls_id = label_sets_.find_or_emplace(tmsr.label_set(), hashval);
    for (const auto& smpl : tmsr.samples()) {
      buffer_.add(ls_id, smpl);
    }
  }

  template <class OutputStream>
  friend OutputStream& operator<<(OutputStream& out, BasicEncoder& wal) {
    auto redundant = wal.encode_segment(out);
    delete redundant;  // to avoid memory leak // TODO: std::unique_ptr?
    return out;
  }

  template <class OutputStream>
  inline __attribute__((always_inline)) Redundant* write(OutputStream& out) {
    return encode_segment(out);
  }

  template <std::ranges::bidirectional_range Iterator, typename OutputStream>
    requires std::is_same_v<typename std::iterator_traits<typename Iterator::iterator>::value_type, Redundant*>
  inline __attribute__((always_inline)) void snapshot(Iterator& it, OutputStream& out) {
    assert(next_encoded_segment_ > 0);
    uint32_t segment = (std::begin(it) != std::end(it)) ? (*std::begin(it))->segment : next_encoded_segment_;
    auto label_sets_checkpoint = (std::begin(it) != std::end(it)) ? (*std::begin(it))->label_sets_checkpoint : label_sets_.checkpoint();

    auto encoders_count = segment == next_encoded_segment_ ? gorilla_.size() : (*it.begin())->encoders_count;
    if (encoders_count > gorilla_.size()) {
      throw BareBones::Exception(0xfd921d184ca372ee, "Encoder's Snapshot %d has more encoders in first redundant (%zd) than already on the Writer (%zd)",
                                 segment, encoders_count, gorilla_.size());
    }

    decltype(gorilla_) encoders;
    encoders.reserve(encoders_count);
    std::ranges::copy(gorilla_ | std::ranges::views::take(encoders_count), std::back_inserter(encoders));
    BareBones::Bitset changed;
    changed.resize(encoders.size());

    uint32_t redundant_segment_id = segment;
    bool has_any_redundant_segment_id = false;
    for (auto& redundant : it) {
      // check that all redundant ids are sequential and throw error if not.
      if (!has_any_redundant_segment_id) {
        has_any_redundant_segment_id = true;
      } else {
        if (not(redundant->segment == redundant_segment_id + 1)) {
          throw BareBones::Exception(0xcddf21b039fe5a18,
                                     "The next redundant in encoders' snapshot (segment_id=%d) must be in order with previous redundant (segment_id=%d)",
                                     redundant->segment, redundant_segment_id + 1);
        }
        redundant_segment_id = redundant->segment;
      }

      for (const auto& encoder_with_id : redundant->encoders) {
        if (encoder_with_id.id >= encoders_count) {
          continue;
        }
        if (!changed[encoder_with_id.id]) {
          changed.set(encoder_with_id.id);
          encoders[encoder_with_id.id] = encoder_with_id.encoder;
        }
      }
    }

    // after cycle there is a last segment id.
    // if have any redundants check that it not equals current redundant_segment_id and next_encoded_segment_ - 1,
    // redundants must be consistent
    if (has_any_redundant_segment_id && redundant_segment_id != next_encoded_segment_ - 1) {
      throw BareBones::Exception(0xc318a18809c8167e,
                                 "The encoder's snapshot doesn't have the latest redundant with expected segment_id=%d, the last redundant has segment_id=%d",
                                 (next_encoded_segment_ - 1), redundant_segment_id);
    }

    BareBones::Vector<BareBones::Encoding::Gorilla::StreamDecoder> decoders;

    // move out the encoders into decoders.
    for (auto&& encoder : encoders) {
      decoders.emplace_back(std::move(encoder));
    }

    auto original_exceptions = out.exceptions();
    auto sg1 = std::experimental::scope_exit([&]() { out.exceptions(original_exceptions); });
    out.exceptions(std::ifstream::failbit | std::ifstream::badbit);

    // write version
    out.put(1);

    // write uuid
    out.write(reinterpret_cast<const char*>(uuid_.as_bytes().data()), 16);

    // write shard ID
    out.write(reinterpret_cast<const char*>(&shard_id_), 2);

    // Write total shards count (in power of two).
    out.write(reinterpret_cast<const char*>(&pow_two_of_total_shards_), 1);

    // write prev segment number
    // It will be used in reader as a last processed segment number (which is 1 less than segment value)
    segment = segment - 1;
    out.write(reinterpret_cast<const char*>(&segment), sizeof(segment));

    // write base label sets snapshot
    label_sets_checkpoint.save(out);
    // TODO: calculate additional segments' deltas, see !155#note_240342.

    // write decoders
    uint32_t decoders_count = decoders.size();
    out.write(reinterpret_cast<const char*>(&decoders_count), sizeof(decoders_count));
    for (auto& decoder : decoders) {
      decoder.save(out);
    }
  }
};

template <class LabelSetsTable = Primitives::SnugComposites::LabelSet::DecodingTable>
class BasicDecoder {
  LabelSetsTable label_sets_;
  BareBones::Vector<BareBones::Encoding::Gorilla::StreamDecoder> gorilla_;

  uuids::uuid uuid_;
  uint32_t last_processed_segment_ = std::numeric_limits<uint32_t>::max();

  Primitives::Timestamp ts_base_;

  BareBones::BitSequence segment_gorilla_ts_bitseq_;
  BareBones::BitSequence segment_gorilla_v_bitseq_;
  BareBones::EncodedSequence<BareBones::Encoding::DeltaRLE<>> segment_ls_id_delta_rle_seq_;
  BareBones::EncodedSequence<BareBones::Encoding::DeltaZigZagRLE<>> segment_ts_delta_rle_seq_;

  BareBones::CRC32 segment_ls_id_crc_;
  BareBones::CRC32 segment_ts_crc_;
  BareBones::CRC32 segment_v_crc_;
  uint16_t shard_id_ = 0;
  uint8_t pow_two_of_total_shards_ = 0;
  int64_t created_at_tsns_ = 0;
  int64_t encoded_at_tsns_ = 0;

  void clear_segment() {
    segment_gorilla_ts_bitseq_.clear();
    segment_gorilla_v_bitseq_.clear();
    segment_ls_id_delta_rle_seq_.clear();
    segment_ts_delta_rle_seq_.clear();
    segment_ls_id_crc_.clear();
    segment_ts_crc_.clear();
    segment_v_crc_.clear();
  }

  uint64_t samples_ = 0;

  template <typename InputStream>
  static auto read_uuid(InputStream& in) {
    assert(in);

    // read uuid
    std::array<char, 16> uuid_bytes;
    in.read(uuid_bytes.data(), 16);
    auto uuid = uuids::uuid(uuid_bytes.begin(), uuid_bytes.end());

    // validate uuid
    if (uuid.is_nil()) {
      throw BareBones::Exception(0xec5e9e3ea3edec11, "Segment has an invalid UUID");
    }
    if (uuid.version() != uuids::uuid_version::random_number_based) {
      // N.B.: UUID version is determined by 6th byte.
      throw BareBones::Exception(0x06b621cb184ad541, "Segment's UUID (%s) version is not supported, only RFC's random_number_based version (0x40) is supported",
                                 uuids::to_string(uuid).c_str());
    }
    if (uuid.variant() != uuids::uuid_variant::rfc) {
      // N.B.: UUID variant is determined by 8th byte.
      throw BareBones::Exception(0x5dc8c27e17e55060, "Segment's UUID (%s) variant is not supported, only RFC-4412 UUIDs are supported",
                                 uuids::to_string(uuid).c_str());
    }

    return uuid;
  }

  template <class InputStream>
  void load_segment(InputStream& in) {
    assert(segment_gorilla_v_bitseq_.empty());

    // read version
    uint8_t version = in.get();

    // return successfully, if stream is empty
    if (in.eof())
      return;

    // check version
    if (version != 1) {
      throw BareBones::Exception(0x3449dc095f9e2f31, "Invalid segment version (%d), only version 1 is supported", version);
    }

    auto original_exceptions = in.exceptions();
    auto sg1 = std::experimental::scope_exit([&]() { in.exceptions(original_exceptions); });
    in.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit);

    // read uuid
    // TODO: More context related to segment is available here. It may be worth to add it
    //       into exception messages.
    auto uuid = read_uuid(in);

    // associate uuid if it's a first segment
    if (last_processed_segment_ + 1 == 0)
      uuid_ = uuid;

    // check uuid
    if (uuid_ != uuid) {
      throw BareBones::Exception(0x4050da9e13900f11, "Input segment's UUID (%s) doesn't match with Decoder's UUID (%s)", uuids::to_string(uuid).c_str(),
                                 uuids::to_string(uuid_).c_str());
    }

    {
      uint16_t shard_id = 0;
      uint8_t pow_two_of_total_shards = 0;

      // read shard ID
      in.read(reinterpret_cast<char*>(&shard_id), sizeof(shard_id));

      // associate shard_id if it's a first segment
      if (last_processed_segment_ + 1 == 0) {
        shard_id_ = shard_id;
      }

      if (shard_id != shard_id_) {
        throw BareBones::Exception(0xcf388325297850a4, "Input segment's shard id (%d) doesn't match with Decoder's shard id (%d)", shard_id, shard_id_);
      }

      // read pow of two of total shards
      in.read(reinterpret_cast<char*>(&pow_two_of_total_shards), sizeof(pow_two_of_total_shards));

      // associate also shards count if it's a first segment
      if (last_processed_segment_ + 1 == 0) {
        pow_two_of_total_shards_ = pow_two_of_total_shards;
      }

      if (pow_two_of_total_shards != pow_two_of_total_shards_) {
        throw BareBones::Exception(0x85a8f764e17983db, "Input segment's shards count (%d) doesn't match with Decoder's shards count (%d)",
                                   pow_two_of_total_shards, pow_two_of_total_shards_);
      }
    }

    // read segment
    uint32_t segment;
    in.read(reinterpret_cast<char*>(&segment), sizeof(segment));

    if (segment != last_processed_segment_ + 1) {
      std::stringstream ss;
      ss << "Unexpected input segment id " << segment << ", expected " << (last_processed_segment_ + 1);
      throw BareBones::Exception(0xfb9b62e957a1ac39, ss.str());
    }

    in.read(reinterpret_cast<char*>(&created_at_tsns_), sizeof(created_at_tsns_));
    in.read(reinterpret_cast<char*>(&encoded_at_tsns_), sizeof(encoded_at_tsns_));

    // read label sets
    label_sets_.load(in);

    try {
      // read ls ids
      in >> segment_ls_id_delta_rle_seq_ >> segment_ls_id_crc_;

      // read ts base
      in.read(reinterpret_cast<char*>(&ts_base_), sizeof(ts_base_));

      // read ts
      if (in.get() == 0) {
        in >> segment_ts_delta_rle_seq_ >> segment_ts_crc_;
      } else {
        in >> segment_gorilla_ts_bitseq_ >> segment_ts_crc_;
      }

      // read values
      in >> segment_gorilla_v_bitseq_ >> segment_v_crc_;
    } catch (...) {
      clear_segment();
      throw;
    }

    ++last_processed_segment_;
    gorilla_.resize(label_sets_.size());
  }

 public:
  // label sets' comparison is expensive, so it must be explicitly compared
  // as a byte streams.
  bool operator==(const BasicDecoder& reader) const noexcept {
    return this->uuid_ == reader.uuid_ && this->last_processed_segment_ == reader.last_processed_segment_ && this->shard_id_ == reader.shard_id_ &&
           this->pow_two_of_total_shards_ == reader.pow_two_of_total_shards_;
  }

  template <typename InputStream>
  void load_snapshot(InputStream& in) {
    assert(segment_gorilla_v_bitseq_.empty());

    // the snapshot must be loaded from first segment! (from review)
    // !155#note_241482
    if (last_processed_segment_ + 1 != 0) {
      throw BareBones::Exception(0x25fa0d279a79b3f6, "Can't load Snapshot into non-empty Decoder");
    }

    // read version
    uint8_t version = in.get();

    // return successfully, if stream is empty
    if (in.eof())
      return;

    // check version
    if (version != 1) {
      throw BareBones::Exception(0xccd8f4f87758ca2f, "Invalid snapshot version (%d), only version 1 is supported", version);
    }

    auto original_exceptions = in.exceptions();
    auto sg1 = std::experimental::scope_exit([&]() { in.exceptions(original_exceptions); });
    in.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit);

    // read uuid
    auto uuid = read_uuid(in);

    // associate uuid, assuming that it's a first segment
    uuid_ = uuid;

    // read shard ID
    in.read(reinterpret_cast<char*>(&shard_id_), sizeof(shard_id_));

    // read pow of two of total shards
    in.read(reinterpret_cast<char*>(&pow_two_of_total_shards_), sizeof(pow_two_of_total_shards_));

    // read segment number
    in.read(reinterpret_cast<char*>(&last_processed_segment_), sizeof(last_processed_segment_));

    // read label sets snapshot
    label_sets_.load(in);

    // read decoders
    uint32_t decoders_count = 0;
    in.read(reinterpret_cast<char*>(&decoders_count), sizeof(decoders_count));
    gorilla_.resize(decoders_count);
    for (auto& decoder : gorilla_) {
      decoder.load(in);
    }
  }

  inline __attribute__((always_inline)) const LabelSetsTable& label_sets() const { return label_sets_; }

  inline __attribute__((always_inline)) uint32_t series() const { return label_sets_.size(); }

  inline __attribute__((always_inline)) const BareBones::Vector<BareBones::Encoding::Gorilla::StreamDecoder>& decoders() const { return gorilla_; }

  /// \Returns Total processed samples count.
  /// \seealso \ref process_segment().
  inline __attribute__((always_inline)) uint64_t samples() const { return samples_; }

  inline __attribute__((always_inline)) uint16_t shard_id() const noexcept { return shard_id_; }

  inline __attribute__((always_inline)) uint8_t pow_two_of_total_shards() const noexcept { return pow_two_of_total_shards_; }

  inline __attribute__((always_inline)) uint32_t last_processed_segment() const { return last_processed_segment_; }

  inline __attribute__((always_inline)) int64_t created_at_tsns() const { return created_at_tsns_; }

  inline __attribute__((always_inline)) int64_t encoded_at_tsns() const { return encoded_at_tsns_; }

  template <class InputStream>
  friend InputStream& operator>>(InputStream& in, BasicDecoder& wal) {
    wal.load_segment(in);
    return in;
  }

  template <class Callback>
    requires std::is_invocable_v<Callback, Primitives::LabelSetID, Primitives::Timestamp, Primitives::Sample::value_type>
  __attribute__((flatten)) void process_segment(Callback func) {
    if (__builtin_expect(segment_gorilla_v_bitseq_.empty(), false))
      return;

    BareBones::CRC32 ls_id_crc;
    BareBones::CRC32 ts_crc;
    BareBones::CRC32 v_crc;

    if (segment_gorilla_ts_bitseq_.empty()) {
      auto ts_i = segment_ts_delta_rle_seq_.begin();
      auto g_v_bitseq_reader = segment_gorilla_v_bitseq_.reader();

      for (Primitives::LabelSetID ls_id : segment_ls_id_delta_rle_seq_) {
        if (__builtin_expect(ls_id >= gorilla_.size(), false)) {
          throw BareBones::Exception(0xf0e57d2a0e5ce7ed, "Error while processing segment LabelSets: Unknown segment's LabelSet's id %d", ls_id);
        }

        if (__builtin_expect(g_v_bitseq_reader.left() == 0, false)) {
          throw BareBones::Exception(0xa5cc1f527d80b20f,
                                     "Decoder %s exhausted label set values data prematurely, but segment processing expects more LabelSets' values",
                                     uuids::to_string(uuid_).c_str());
        }

        auto& g = gorilla_[ls_id];

        g.decode(*ts_i, g_v_bitseq_reader);

        ls_id_crc << ls_id;
        ts_crc << (g.last_timestamp() + ts_base_);
        v_crc << g.last_value();

        ++ts_i;
        ++samples_;

        func(ls_id, g.last_timestamp() + ts_base_, g.last_value());
      }

      // there are remaining timestamps in Decoder/segment (ls_id), which is unexpected.
      if (ts_i != segment_ts_delta_rle_seq_.end()) {
        std::stringstream ss;

        ss << "Decoder " << uuid_
           << " got an error after processing segment LabelSets: segment ls_id timestamps counts mismatch, there are "
              "remaining timestamp data";
        throw BareBones::Exception(0x6b534297844a47c9, ss.str());
      }

      if (g_v_bitseq_reader.left() != 0) {
        std::stringstream ss;

        ss << "Decoder" << uuid_ << " got an error after processing segment LabelSets: segment ls_id values (" << g_v_bitseq_reader.left()
           << ") and Decoder's values (" << segment_gorilla_v_bitseq_.size() << ") counts mismatch, there are remaining values data";

        throw BareBones::Exception(0x934f6048d089ae64, ss.str());
      }
    } else {
      // process non-empty ts
      auto g_ts_bitseq_reader = segment_gorilla_ts_bitseq_.reader();
      auto g_v_bitseq_reader = segment_gorilla_v_bitseq_.reader();

      for (Primitives::LabelSetID ls_id : segment_ls_id_delta_rle_seq_) {
        // same checks as in prev. ls_id parsing.
        // TODO: Merge it?
        if (__builtin_expect(ls_id >= gorilla_.size(), false)) {
          std::stringstream ss;
          ss << "Decoder " << uuid_ << " got an error while processing segment LabelSets: Unknown segment's LabelSet's id " << ls_id;
          throw BareBones::Exception(0x19884e9893440316, ss.str());
        }

        if (__builtin_expect(g_ts_bitseq_reader.left() == 0, false)) {
          std::stringstream ss;
          ss << "Decoder " << uuid_ << " exhausted label set values data prematurely, but segment processing expects more LabelSets' timestamps";
          throw BareBones::Exception(0xf837b80ba182e441, ss.str());
        }

        if (__builtin_expect(g_v_bitseq_reader.left() == 0, false)) {
          std::stringstream ss;
          ss << "Decoder " << uuid_ << " exhausted label set values data prematurely, but segment processing expects more LabelSets' values";
          throw BareBones::Exception(0xe667122e5d11ba4c, ss.str());
        }

        auto& g = gorilla_[ls_id];

        g.decode(g_ts_bitseq_reader, g_v_bitseq_reader);

        ls_id_crc << ls_id;
        ts_crc << (g.last_timestamp() + ts_base_);
        v_crc << g.last_value();

        ++samples_;

        func(ls_id, g.last_timestamp() + ts_base_, g.last_value());
      }

      if (g_ts_bitseq_reader.left() != 0) {
        std::stringstream ss;
        ss << "Decoder " << uuid_ << "got error after parsing LabelSets: there are more remaining timestamps data";
        throw BareBones::Exception(0x5352e912e73554c1, ss.str());
      }

      if (g_v_bitseq_reader.left() != 0) {
        std::stringstream ss;
        ss << "Decoder " << uuid_ << "got error after parsing LabelSets: there are more remaining values data";
        throw BareBones::Exception(0x71811aa3dc793602, ss.str());
      }
    }

    if (ls_id_crc != segment_ls_id_crc_) {
      std::stringstream ss;
      ss << "Decoder " << uuid_ << "got error: CRC for LabelSet's ids mismatch: Decoder ls_id CRC: " << segment_ls_id_crc_
         << ", segment ls_id CRC: " << ls_id_crc;
      throw BareBones::Exception(0x6ea4e8b039aea0e8, ss.str());
    }
    if (ts_crc != segment_ts_crc_) {
      std::stringstream ss;
      ss << "Decoder " << uuid_ << "got error: CRC for LabelSet's timestamps mismatch: Decoder ts CRC: " << segment_ts_crc_ << ", segment ts CRC: " << ts_crc;
      throw BareBones::Exception(0x0fd1fbf569f6c3c5, ss.str());
    }
    if (v_crc != segment_v_crc_) {
      std::stringstream ss;
      ss << "Decoder " << uuid_ << "got error: CRC for LabelSet's values mismatch: Decoder v CRC: " << segment_v_crc_ << ", segment v CRC: " << v_crc;
      throw BareBones::Exception(0x0ee2b199218aaf7d, ss.str());
    }

    clear_segment();
  }

  using label_set_type = const typename LabelSetsTable::value_type&;

  template <class Callback>
    requires std::is_invocable_v<Callback, label_set_type, Primitives::Timestamp, Primitives::Sample::value_type>
  __attribute__((flatten)) void process_segment(Callback func) {
    process_segment([&](Primitives::LabelSetID ls_id, Primitives::Timestamp ts, Primitives::Sample::value_type v) {
      const auto& label_set = label_sets_[ls_id];

      func(label_set, ts, v);
    });
  }

  using timeseries_type = const Primitives::BasicTimeseries<typename LabelSetsTable::value_type*>&;

  template <class Callback>
    requires std::is_invocable_v<Callback, timeseries_type>
  __attribute__((flatten)) void process_segment(Callback func) {
    Primitives::BasicTimeseries<typename LabelSetsTable::value_type::composite_type*> timeseries;

    typename LabelSetsTable::value_type::composite_type last_ls;
    Primitives::LabelSetID last_ls_id = std::numeric_limits<Primitives::LabelSetID>::max();

    process_segment([&](Primitives::LabelSetID ls_id, Primitives::Timestamp ts, Primitives::Sample::value_type v) {
      if (ls_id != last_ls_id) {
        if (last_ls_id != std::numeric_limits<Primitives::LabelSetID>::max()) {
          func(timeseries);
        }

        last_ls = label_sets_[ls_id];
        timeseries.set_label_set(&last_ls);
        timeseries.samples().resize(0);
        last_ls_id = ls_id;
      }

      timeseries.samples().push_back(Primitives::Sample(ts, v));
    });

    if (last_ls_id != std::numeric_limits<Primitives::LabelSetID>::max()) {
      func(timeseries);
    }
  }
};

using Writer = BasicEncoder<Primitives::SnugComposites::LabelSet::EncodingBimap>;
using Reader = BasicDecoder<Primitives::SnugComposites::LabelSet::DecodingTable>;
}  // namespace PromPP::WAL
