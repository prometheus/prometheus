#pragma once

#include <fstream>
#include <iterator>
#include <limits>
#include <type_traits>
#include <utility>

#include <scope_exit.h>

#include "bit_sequence.h"
#include "stream_v_byte.h"
#include "streams.h"
#include "zigzag.h"

namespace BareBones {
namespace Encoding {
template <class Container = StreamVByte::Sequence<StreamVByte::Codec0124Frequent0>>
class RLE {
 public:
  using DataSequence = Container;

  class Encoder {
    using value_type = typename DataSequence::value_type;

    value_type count_ = std::numeric_limits<value_type>::max();
    value_type last_;

   public:
    inline __attribute__((always_inline)) Encoder() noexcept = default;
    Encoder(const Encoder&) = delete;
    Encoder& operator=(const Encoder&) = delete;

    inline __attribute__((always_inline)) Encoder(Encoder&& o) noexcept : count_(o.count_), last_(o.last_) { o.clear(); }

    inline __attribute__((always_inline)) Encoder& operator=(Encoder&& o) noexcept {
      count_ = o.count_;
      last_ = o.last_;
      o.clear();
      return *this;
    }

    template <std::output_iterator<value_type> IteratorType>
    inline __attribute__((always_inline)) void encode(value_type val, IteratorType& i) noexcept {
      // assume last_ = val on first call
      if (count_ == std::numeric_limits<value_type>::max())
        last_ = val;

      if (val == last_) {
        ++count_;

        // check for overflow
        if (count_ == std::numeric_limits<value_type>::max() - 1) {
          *i++ = last_;
          *i++ = count_;
          count_ = 0;
        }
      } else {
        *i++ = last_;
        *i++ = count_;
        last_ = val;
        count_ = 0;  // use 0 to encode 1 occurrence
      }
    }

    inline __attribute__((always_inline)) void clear() noexcept { count_ = std::numeric_limits<value_type>::max(); }
    inline __attribute__((always_inline)) bool empty() noexcept { return count_ == std::numeric_limits<value_type>::max(); }

    template <std::output_iterator<value_type> IteratorType>
    inline __attribute__((always_inline)) void flush(IteratorType& i) noexcept {
      if (count_ != std::numeric_limits<value_type>::max()) {
        *i++ = last_;
        *i++ = count_;
        clear();
      }
    }

    inline __attribute__((always_inline)) value_type count() const noexcept { return count_; }
    inline __attribute__((always_inline)) value_type last() const noexcept { return last_; }
  };

  class Decoder {
    using value_type = typename DataSequence::value_type;

    value_type count_ = 0;
    value_type last_;
    bool decoding_from_encoder_buffer_ = false;

   public:
    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    inline __attribute__((always_inline)) Decoder(IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) noexcept {
      next(begin, end, encoder);
    }

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    inline __attribute__((always_inline)) value_type decode(const IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) const noexcept {
      return last_;
    }

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    inline __attribute__((always_inline)) void next(IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) noexcept {
      // range check
      assert(count_ != std::numeric_limits<value_type>::max());

      --count_;

      if (count_ == std::numeric_limits<value_type>::max()) {
        if (begin != end) {
          last_ = *begin++;
          assert(begin != end);

          if (__builtin_expect(begin == end, false))
            // that's not a normal situation, but we allow one missing count at the end
            // and treat it as if it would have been zero to make all the code exceptions free
            count_ = 0;
          else
            count_ = *begin++;
        } else if (!decoding_from_encoder_buffer_) {
          last_ = encoder.last();
          count_ = encoder.count();
          decoding_from_encoder_buffer_ = true;
        }
      }
    }

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    inline __attribute__((always_inline)) bool is_finished(const IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) const noexcept {
      return begin == end && count_ == std::numeric_limits<value_type>::max() &&
             (decoding_from_encoder_buffer_ || encoder.count() == std::numeric_limits<value_type>::max());
    }
  };
};

template <class Container = StreamVByte::Sequence<StreamVByte::Codec0124Frequent0>>
class DeltaRLE {
 public:
  using DataSequence = Container;

  class Encoder : public RLE<DataSequence>::Encoder {
    using value_type = typename DataSequence::value_type;

    value_type last_ = 0;
    using Base = typename RLE<DataSequence>::Encoder;

   public:
    template <std::output_iterator<value_type> IteratorType>
    inline __attribute__((always_inline)) void encode(value_type val, IteratorType& i) noexcept {
      assert(val >= last_);
      Base::encode(val - last_, i);
      last_ = val;
    }

    inline __attribute__((always_inline)) void clear() noexcept {
      Base::clear();
      last_ = 0;
    }
    inline __attribute__((always_inline)) bool empty() noexcept { return last_ == 0 && Base::empty(); }
  };

  class Decoder : public RLE<DataSequence>::Decoder {
    using value_type = typename DataSequence::value_type;

    value_type last_ = 0;
    using Base = typename RLE<DataSequence>::Decoder;

   public:
    using Base::Base;
    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    inline __attribute__((always_inline)) value_type decode(const IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) const noexcept {
      return last_ + Base::decode(begin, end, encoder);
    }

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    inline __attribute__((always_inline)) void next(IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) noexcept {
      last_ += Base::decode(begin, end, encoder);
      Base::next(begin, end, encoder);
    }
  };
};

template <class Container = StreamVByte::Sequence<StreamVByte::Codec0124Frequent0>>
class DeltaZigZagRLE {
 public:
  using DataSequence = Container;

  class Encoder : public RLE<DataSequence>::Encoder {
    using value_type = typename DataSequence::value_type;
    typedef typename std::make_signed<value_type>::type int_type;

    value_type last_ = 0;
    using Base = typename RLE<DataSequence>::Encoder;

   public:
    template <std::output_iterator<value_type> IteratorType>
    inline __attribute__((always_inline)) void encode(value_type val, IteratorType& i) noexcept {
      Base::encode(ZigZag::encode(std::bit_cast<int_type>(val) - std::bit_cast<int_type>(last_)), i);
      last_ = val;
    }

    inline __attribute__((always_inline)) void clear() noexcept {
      Base::clear();
      last_ = 0;
    }
    inline __attribute__((always_inline)) bool empty() noexcept { return last_ == 0 && Base::empty(); }
  };

  class Decoder : public RLE<DataSequence>::Decoder {
    using value_type = typename DataSequence::value_type;

    value_type last_ = 0;
    using Base = typename RLE<DataSequence>::Decoder;

   public:
    using Base::Base;
    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    inline __attribute__((always_inline)) value_type decode(const IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) const noexcept {
      return last_ + ZigZag::decode(Base::decode(begin, end, encoder));
    }

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    inline __attribute__((always_inline)) void next(IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) noexcept {
      last_ += ZigZag::decode(Base::decode(begin, end, encoder));
      Base::next(begin, end, encoder);
    }
  };
};

template <typename E>
struct id;
template <class DataSequence>
struct id<RLE<DataSequence>> : std::integral_constant<uint8_t, 0 + (sizeof(typename DataSequence::value_type) == 8) * 3> {};
template <class DataSequence>
struct id<DeltaRLE<DataSequence>> : std::integral_constant<uint8_t, 1 + (sizeof(typename DataSequence::value_type) == 8) * 3> {};
template <class DataSequence>
struct id<DeltaZigZagRLE<DataSequence>> : std::integral_constant<uint8_t, 2 + (sizeof(typename DataSequence::value_type) == 8) * 3> {};
}  // namespace Encoding

template <class E, class DataSequence = typename E::DataSequence>
class EncodedSequence {
  typename E::Encoder encoder_;

  DataSequence data_;
  std::back_insert_iterator<DataSequence> data_back_inserter_;

 public:
  using value_type = typename DataSequence::value_type;

  inline __attribute__((always_inline)) EncodedSequence() noexcept : data_back_inserter_(std::back_inserter(data_)) {}

  EncodedSequence(const EncodedSequence&) = delete;
  EncodedSequence& operator=(const EncodedSequence&) = delete;

  inline __attribute__((always_inline)) EncodedSequence(EncodedSequence&& o) noexcept
      : encoder_(std::move(o.encoder_)), data_(std::move(o.data_)), data_back_inserter_(std::back_inserter(data_)) {
    o.data_back_inserter_ = std::back_inserter(o.data_);
  }

  inline __attribute__((always_inline)) EncodedSequence& operator=(EncodedSequence&& o) noexcept {
    encoder_ = std::move(o.encoder_);
    data_ = std::move(o.data_);
    data_back_inserter_ = std::back_inserter(data_);
    o.data_back_inserter_ = std::back_inserter(o.data_);
    return *this;
  }

  inline __attribute__((always_inline)) void push_back(value_type val) noexcept { encoder_.encode(val, data_back_inserter_); }

  inline __attribute__((always_inline)) void flush() noexcept { encoder_.flush(data_back_inserter_); }

  inline __attribute__((always_inline)) void clear() noexcept {
    encoder_.clear();
    data_.clear();
  }

  inline __attribute__((always_inline)) const DataSequence& data() const noexcept { return data_; }

  class IteratorSentinel {};

  class Iterator {
    typename DataSequence::const_iterator begin_;
    typename DataSequence::sentinel end_;
    const typename E::Encoder* encoder_;
    typename E::Decoder decoder_;

   public:
    using iterator_category = std::input_iterator_tag;
    using value_type = typename DataSequence::value_type;
    using difference_type = std::ptrdiff_t;

    inline __attribute__((always_inline))
    Iterator(typename DataSequence::const_iterator begin, typename DataSequence::sentinel end, const typename E::Encoder* encoder) noexcept
        : begin_(begin), end_(end), encoder_(encoder), decoder_(begin_, end_, *encoder_) {}

    inline __attribute__((always_inline)) Iterator& operator++() noexcept {
      decoder_.next(begin_, end_, *encoder_);
      return *this;
    }

    inline __attribute__((always_inline)) Iterator operator++(int) noexcept {
      Iterator retval = *this;
      ++(*this);
      return retval;
    }

    inline __attribute__((always_inline)) bool operator==(const IteratorSentinel& other) const noexcept {
      return decoder_.is_finished(begin_, end_, *encoder_);
    }

    inline __attribute__((always_inline)) value_type operator*() const noexcept { return decoder_.decode(begin_, end_, *encoder_); }
  };

  using iterator_type = Iterator;
  using const_iteartor_type = Iterator;
  using sentinel = IteratorSentinel;

  inline __attribute__((always_inline)) auto begin() const noexcept { return Iterator(data_.begin(), data_.end(), &encoder_); }

  inline __attribute__((always_inline)) auto end() const noexcept { return IteratorSentinel(); }

  inline __attribute__((always_inline)) size_t save_size() noexcept {
    flush();

    // version is written and read by methods put() and get() and they write and read 1 byte
    return 1 + sizeof(Encoding::id<E>::value) + data_.save_size();
  }

  template <OutputStream S>
  friend S& operator<<(S& out, EncodedSequence& seq) {
    seq.flush();

    auto original_exceptions = out.exceptions();
    auto sg1 = std::experimental::scope_exit([&]() { out.exceptions(original_exceptions); });
    out.exceptions(std::ifstream::failbit | std::ifstream::badbit);

    // write version
    out.put(1);

    // write encoding id
    out.write(reinterpret_cast<const char*>(&Encoding::id<E>::value), sizeof(Encoding::id<E>::value));

    // write data
    out << seq.data_;

    return out;
  }

  template <class InputStream>
  friend InputStream& operator>>(InputStream& in, EncodedSequence& seq) {
    assert(seq.data_.empty());
    assert(seq.encoder_.empty());
    auto sg1 = std::experimental::scope_fail([&]() { seq.clear(); });

    // read version
    uint8_t version = in.get();

    // return successfully, if stream is empty
    if (in.eof())
      return in;

    // check version
    if (version != 1) {
      char buf[100];
      std::snprintf(buf, sizeof(buf), "unknown version %d", version);
      throw std::logic_error(buf);
    }

    auto original_exceptions = in.exceptions();
    auto sg2 = std::experimental::scope_exit([&]() { in.exceptions(original_exceptions); });
    in.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit);

    // read encoding id
    typename std::remove_const<decltype(Encoding::id<E>::value)>::type encoding_id;
    in.read(reinterpret_cast<char*>(&encoding_id), sizeof(encoding_id));
    if (encoding_id != Encoding::id<E>::value)
      throw std::runtime_error("J: Meaningful message supposed to be here!");

    // read data
    in >> seq.data_;

    return in;
  }
};

}  // namespace BareBones
