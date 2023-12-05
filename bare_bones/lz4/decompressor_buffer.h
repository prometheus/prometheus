#pragma once

#include <memory>
#include <string_view>

namespace BareBones::lz4 {

class DecompressorBuffer {
 public:
  struct ShrinkParameters {
    const double shrink_ratio;
    const double threshold_size_shrink_ratio;
  };

 public:
  explicit DecompressorBuffer(const ShrinkParameters& shrink_parameters) : shrink_parameters_(shrink_parameters) {}

  void allocate(size_t size) {
    auto size_history_avg = size_history_.avg();
    size_history_.push(size);

    if (size_history_avg < get_threshold_size_to_shrink()) {
      if (auto size_to_shrink = get_size_to_shrink(); size_to_shrink >= size) {
        buffer_.resize(size_to_shrink);
        return;
      }
    }

    resize_if_needed(size);
  }

  void reallocate(size_t size) {
    buffer_.reallocate(size);
    size_history_.set_current(size);
  }

  [[nodiscard]] size_t size() const noexcept { return buffer_.size(); }
  [[nodiscard]] char* data() noexcept { return buffer_.data(); }

  [[nodiscard]] std::string_view view(size_t size) const noexcept { return {buffer_.data(), size}; }
  [[nodiscard]] std::string_view view() const noexcept { return {buffer_.data(), buffer_.size()}; }

 private:
  void resize_if_needed(size_t size) {
    if (buffer_.size() < size) {
      buffer_.resize(size);
    }
  }

  [[nodiscard]] size_t get_threshold_size_to_shrink() const noexcept { return calculate_size_ratio(shrink_parameters_.threshold_size_shrink_ratio); }
  [[nodiscard]] size_t get_size_to_shrink() const noexcept { return buffer_.size() - calculate_size_ratio(shrink_parameters_.shrink_ratio); }

  [[nodiscard]] size_t calculate_size_ratio(double percent) const noexcept { return static_cast<size_t>(buffer_.size() * percent); }

 private:
  class SizeHistory {
   public:
    void push(size_t value) {
      values[1] = values[0];
      set_current(value);
    }

    void set_current(size_t value) { values[0] = value; }

    [[nodiscard]] size_t avg() const noexcept { return (values[0] + values[1]) / std::size(values); }

   private:
    size_t values[2]{};
  };

  class Buffer {
   public:
    void resize(size_t size) {
      buffer_.reset(new char[size]);
      size_ = size;
    }

    void reallocate(size_t size) {
      if (size > size_) {
        std::unique_ptr<char[]> new_buffer{new char[size]};
        memcpy(new_buffer.get(), buffer_.get(), size_);
        buffer_ = std::move(new_buffer);
        size_ = size;
      }
    }

    [[nodiscard]] size_t size() const noexcept { return size_; }
    [[nodiscard]] char* data() noexcept { return buffer_.get(); }
    [[nodiscard]] char* data() const noexcept { return buffer_.get(); }

   private:
    std::unique_ptr<char[]> buffer_;
    size_t size_{};
  };

 private:
  const ShrinkParameters shrink_parameters_;
  Buffer buffer_;
  SizeHistory size_history_;
};

}  // namespace BareBones::lz4
