#pragma once

#include <array>

#include "bare_bones/bit_sequence.h"

namespace series_data::encoder {

static inline constexpr std::array kAllocationSizesTable = {
    BareBones::AllocationSize(0),    BareBones::AllocationSize(32),   BareBones::AllocationSize(64),   BareBones::AllocationSize(96),
    BareBones::AllocationSize(128),  BareBones::AllocationSize(192),  BareBones::AllocationSize(256),  BareBones::AllocationSize(384),
    BareBones::AllocationSize(512),  BareBones::AllocationSize(640),  BareBones::AllocationSize(768),  BareBones::AllocationSize(1024),
    BareBones::AllocationSize(1152), BareBones::AllocationSize(1280), BareBones::AllocationSize(1408), BareBones::AllocationSize(1536),
    BareBones::AllocationSize(2048), BareBones::AllocationSize(2176), BareBones::AllocationSize(2304), BareBones::AllocationSize(2432),
    BareBones::AllocationSize(2560), BareBones::AllocationSize(3076), BareBones::AllocationSize(3584), BareBones::AllocationSize(4096),
    BareBones::AllocationSize(4608), BareBones::AllocationSize(5120), BareBones::AllocationSize(5632), BareBones::AllocationSize(6144),
    BareBones::AllocationSize(6656), BareBones::AllocationSize(7168), BareBones::AllocationSize(7680), BareBones::AllocationSize(8192),
};

}