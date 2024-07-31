#include "primitives_lss.h"
#include "_helpers.hpp"

#include "primitives/snug_composites.h"

//
// LSS EncodingBimap
//

/**
 * @brief Construct a new Primitives label sets.
 *
 * @param res {
 *     lss uintptr // pointer to constructed label sets;
 * }
 */
extern "C" void prompp_primitives_lss_ctor(void* res) {
  struct Result {
    PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap* lss;
  };

  Result* out = new (res) Result();

  out->lss = new PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap();
}

/**
 * @brief Destroy Primitives label sets.
 *
 * @param args {
 *     lss uintptr // pointer of label sets;
 * }
 */
extern "C" void prompp_primitives_lss_dtor(void* args) {
  struct Arguments {
    PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap* lss;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->lss;
}

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr             // pointer to constructed label sets;
 * }
 *
 * @param res {
 *     allocated_memory uint64 // size of allocated memory for label sets;
 * }
 */
extern "C" void prompp_primitives_lss_allocated_memory(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap* lss;
  };
  struct Result {
    size_t allocated_memory;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  out->allocated_memory = in->lss->allocated_memory();
}

//
// LSS OrderedEncodingBimap
//

/**
 * @brief Construct a new Primitives ordered label sets.
 *
 * @param res {
 *     orderedLss uintptr // pointer to constructed ordered label sets;
 * }
 */
extern "C" void prompp_primitives_ordered_lss_ctor(void* res) {
  struct Result {
    PromPP::Primitives::SnugComposites::LabelSet::OrderedEncodingBimap* lss;
  };

  Result* out = new (res) Result();

  out->lss = new PromPP::Primitives::SnugComposites::LabelSet::OrderedEncodingBimap();
}

/**
 * @brief Destroy Primitives ordered label sets.
 *
 * @param args {
 *     orderedLss uintptr // pointer of ordered label sets;
 * }
 */
extern "C" void prompp_primitives_ordered_lss_dtor(void* args) {
  struct Arguments {
    PromPP::Primitives::SnugComposites::LabelSet::OrderedEncodingBimap* lss;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->lss;
}

/**
 * @brief return size of allocated memory for ordered label sets.
 *
 * @param args {
 *     orderedLss uintptr     // pointer to constructed ordered label sets;
 * }
 *
 * @param res {
 *     allocatedMemory uint64 // size of allocated memory for ordered label sets;
 * }
 */
extern "C" void prompp_primitives_ordered_lss_allocated_memory(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::SnugComposites::LabelSet::OrderedEncodingBimap* lss;
  };
  struct Result {
    size_t allocated_memory;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  out->allocated_memory = in->lss->allocated_memory();
}
