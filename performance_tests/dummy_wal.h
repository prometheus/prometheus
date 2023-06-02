#include <lz4_stream.h>
#include <parallel_hashmap/phmap_utils.h>
#include <cassert>
#include <chrono>
#include <fstream>
#include <future>
#include <iomanip>
#include <iostream>
#include <span>
#include <string>
#include <thread>
#include <vector>

#define XXH_INLINE_ALL
#include "third_party/xxhash/xxhash.h"

class DummyWal {
 public:
  class Timeseries;

  class LabelSet {
    // TODO uint32_t
    std::span<uint16_t> word_positions;
    std::string_view text_data;
    friend struct Timeseries;

   public:
    bool operator==(const LabelSet& o) const {
      return std::equal(std::begin(word_positions), std::end(word_positions), std::begin(o.word_positions), std::end(o.word_positions)) &&
             text_data == o.text_data;
    }

    friend size_t hash_value(const LabelSet& label_set) {
      size_t res = 0;
      for (const auto& [label_name, label_value] : label_set) {
        res = XXH3_64bits_withSeed(label_name.data(), label_name.size(), res) ^ XXH3_64bits_withSeed(label_value.data(), label_value.size(), res);
      }
      return res;
    }

    uint32_t size() const { return word_positions.size() / 2; }

    class IteratorBase {
     protected:
      uint32_t i_;
      uint32_t t_;
      const LabelSet* label_set_;

      IteratorBase(const LabelSet* label_set = 0, uint32_t i = 0) : i_(i), t_(0), label_set_(label_set) { assert(i_ == 0 || i_ == label_set->size()); }

      void next_impl() {
        t_ += label_set_->word_positions[i_ * 2] + label_set_->word_positions[i_ * 2 + 1];
        i_++;
      }
    };

    class Iterator : public IteratorBase {
      friend struct LabelSet;

     public:
      using iterator_category = std::forward_iterator_tag;
      using value_type = std::pair<std::string_view, std::string_view>;
      using difference_type = std::ptrdiff_t;

      using IteratorBase::IteratorBase;
      Iterator& operator++() {
        next_impl();
        return *this;
      }
      Iterator operator++(int) {
        Iterator retval = *this;
        next_impl();
        return retval;
      }
      bool operator==(Iterator other) const { return i_ == other.i_; }
      value_type operator*() const {
        return std::make_pair(std::string_view(label_set_->text_data.data() + t_, label_set_->word_positions[i_ * 2]),
                              std::string_view(label_set_->text_data.data() + t_ + label_set_->word_positions[i_ * 2], label_set_->word_positions[i_ * 2 + 1]));
      }
    };

    Iterator begin() const { return Iterator(this, 0); }
    Iterator end() const { return Iterator(this, word_positions.size() / 2); }

    class Names {
      const LabelSet* label_set_;

      friend class LabelSet;

      Names(const LabelSet* label_set) : label_set_(label_set) {}

     public:
      class Iterator : public IteratorBase {
        friend class Names;
        using IteratorBase::IteratorBase;

       public:
        using iterator_category = std::forward_iterator_tag;
        using value_type = std::string_view;
        using difference_type = std::ptrdiff_t;

        Iterator& operator++() {
          next_impl();
          return *this;
        }
        Iterator operator++(int) {
          Iterator retval = *this;
          next_impl();
          return retval;
        }
        bool operator==(Iterator other) const { return i_ == other.i_; }
        bool operator!=(Iterator other) const { return !(*this == other); }
        value_type operator*() const { return std::string_view(label_set_->text_data.data() + t_, label_set_->word_positions[i_ * 2]); }
      };

      Iterator begin() const { return Iterator(label_set_, 0); }
      Iterator end() const { return Iterator(label_set_, label_set_->size()); }

      friend size_t hash_value(const Names& lsns) {
        size_t res = 0;
        for (const auto& label_name : lsns) {
          res = XXH3_64bits_withSeed(label_name.data(), label_name.size(), res);
        }
        return res;
      }

      uint32_t size() const { return label_set_->size(); }
    };
    Names names() const { return Names(this); }
  };

  class Sample {
    int64_t timestamp_;
    double value_;

    friend class Timeseries;

   public:
    int64_t timestamp() const { return timestamp_; }
    double value() const { return value_; }

    template <class T>
    inline __attribute__((always_inline)) Sample& operator=(const T& s) {
      timestamp_ = s.timestamp();
      value_ = s.value();
      return *this;
    }

    inline __attribute__((always_inline)) Sample(int64_t timestamp, double value) : timestamp_(timestamp), value_(value) {}

    template <class T>
    inline __attribute__((always_inline)) Sample(const T& s) : timestamp_(s.timestamp()), value_(s.value()) {}

    inline __attribute__((always_inline)) Sample() = default;
  };

  class Timeseries {
    std::array<Sample, 1> samples_;
    LabelSet label_set_;

   public:
    const LabelSet& label_set() const { return label_set_; }
    const std::array<Sample, 1>& samples() const { return samples_; }

    inline char* read_from_bytes(char* bufi) {
      auto bufi_orig = bufi;

      samples_[0].timestamp_ = *reinterpret_cast<int64_t*>(bufi);
      bufi += sizeof(int64_t);

      samples_[0].value_ = *reinterpret_cast<double*>(bufi);
      bufi += sizeof(double);

      label_set_.word_positions = std::span<uint16_t>(reinterpret_cast<uint16_t*>(bufi + sizeof(uint16_t)), *reinterpret_cast<uint16_t*>(bufi));

      bufi += sizeof(uint16_t) + label_set_.word_positions.size() * sizeof(uint16_t);

      label_set_.text_data = std::string_view(bufi + sizeof(uint16_t), *reinterpret_cast<uint16_t*>(bufi));
      bufi += sizeof(uint16_t) + label_set_.text_data.size();

      // allign 8
      auto l = (bufi - bufi_orig) % 8;
      bufi += l != 0 ? 8 - l : 0;

      return bufi;
    }
  };

 private:
  uint64_t cnt_ = 0;

  std::ifstream file_;
  lz4_stream::istream in_;
  enum { A, B } a_or_b_ = A;
  std::vector<char> buf_a_;
  std::vector<char> buf_b_;
  std::future<bool> reader_;

  char* bufi_;
  uint64_t buf_cnt_ = 0;

  bool do_read() {
    if (in_.eof())
      return false;

    if (a_or_b_ == A) {
      in_.read(buf_a_.data(), buf_a_.size());
    } else {
      in_.read(buf_b_.data(), buf_b_.size());
    }

    if (in_.bad()) {
      throw std::runtime_error("bad lz4 stream");
    }

    return true;
  }

 public:
  DummyWal(const std::string& file_path) : file_(file_path, std::ios_base::in | std::ios_base::binary), in_(file_) {
    if (!file_.is_open()) {
      throw std::runtime_error("failed to open file '" + file_path + "'");
    }
    buf_a_.resize(16 * 1024 * 1024);
    buf_b_.resize(16 * 1024 * 1024);
    reader_ = std::async(&DummyWal::do_read, this);
  }

  inline bool read_next_segment() {
    if (!reader_.get())
      return false;

    bufi_ = a_or_b_ == A ? buf_a_.data() : buf_b_.data();
    buf_cnt_ = *reinterpret_cast<uint64_t*>(bufi_);
    bufi_ += sizeof(uint64_t);

    a_or_b_ = a_or_b_ == A ? B : A;
    reader_ = std::async(&DummyWal::do_read, this);

    return true;
  }

  inline bool read_next(Timeseries& tmsr) {
    if (buf_cnt_ == 0)
      return false;

    bufi_ = tmsr.read_from_bytes(bufi_);

    buf_cnt_--;
    cnt_++;

    return true;
  }

  uint64_t cnt() const { return cnt_; }
};
