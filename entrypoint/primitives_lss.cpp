#include "primitives_lss.h"
#include "_helpers.hpp"
#include "lss.hpp"

extern "C" void prompp_primitives_lss_ctor(void* args, void* res) {
  struct Arguments {
    entrypoint::LssType lss_type;
  };
  struct Result {
    entrypoint::LssVariantPtr lss;
  };

  new (res) Result{.lss = create_lss(reinterpret_cast<Arguments*>(args)->lss_type)};
}

extern "C" void prompp_primitives_lss_dtor(void* args) {
  struct Arguments {
    entrypoint::LssVariantPtr lss;
  };

  reinterpret_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_primitives_lss_allocated_memory(void* args, void* res) {
  struct Arguments {
    entrypoint::LssVariantPtr lss;
  };
  struct Result {
    uint64_t allocated_memory;
  };

  std::visit([res](const auto& lss) PROMPP_LAMBDA_INLINE { new (res) Result{.allocated_memory = lss.allocated_memory()}; },
             *reinterpret_cast<Arguments*>(args)->lss);
}
