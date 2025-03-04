#pragma once

#include "snug_composites_filaments.h"

namespace PromPP::Primitives::SnugComposites {

namespace Symbol {

template <template <class> class Vector>
using DecodingTable = BareBones::SnugComposite::DecodingTable<Filaments::Symbol, Vector>;

template <template <class> class Vector>
using EncodingBimap = BareBones::SnugComposite::EncodingBimap<Filaments::Symbol, Vector>;

template <template <class> class Vector>
using ParallelEncodingBimap = BareBones::SnugComposite::ParallelEncodingBimap<Filaments::Symbol, Vector>;

template <template <class> class Vector>
using OrderedEncodingBimap = BareBones::SnugComposite::OrderedEncodingBimap<Filaments::Symbol, Vector>;

template <template <class> class Vector>
using EncodingBimapWithOrderedAccess = BareBones::SnugComposite::EncodingBimapWithOrderedAccess<Filaments::Symbol, Vector>;

template <template <class> class Vector>
using OrderedDecodingTable = BareBones::SnugComposite::OrderedDecodingTable<Filaments::Symbol, Vector>;

}  // namespace Symbol

namespace LabelNameSet {

template <template <class> class Vector>
using DecodingTableFilament = Filaments::LabelNameSet<Symbol::DecodingTable, Vector>;

template <template <class> class Vector>
using DecodingTable = BareBones::SnugComposite::DecodingTable<DecodingTableFilament, Vector>;

template <template <class> class Vector>
using EncodingBimapFilament = Filaments::LabelNameSet<Symbol::EncodingBimap, Vector>;

template <template <class> class Vector>
using EncodingBimap = BareBones::SnugComposite::EncodingBimap<EncodingBimapFilament, Vector>;

template <template <class> class Vector>
using OrderedEncodingBimapFilament = Filaments::LabelNameSet<Symbol::OrderedEncodingBimap, Vector>;

template <template <class> class Vector>
using OrderedEncodingBimap = BareBones::SnugComposite::OrderedEncodingBimap<OrderedEncodingBimapFilament, Vector>;

template <template <class> class Vector>
using EncodingBimapWithOrderedAccessFilament = Filaments::LabelNameSet<Symbol::EncodingBimapWithOrderedAccess, Vector>;

template <template <class> class Vector>
using EncodingBimapWithOrderedAccess = BareBones::SnugComposite::EncodingBimapWithOrderedAccess<EncodingBimapWithOrderedAccessFilament, Vector>;

template <template <class> class Vector>
using EncodingBimapWithOrderedAccessToSymbolsFilament = Filaments::LabelNameSet<Symbol::EncodingBimapWithOrderedAccess, Vector>;

template <template <class> class Vector>
using EncodingBimapWithOrderedAccessToSymbols = BareBones::SnugComposite::EncodingBimap<EncodingBimapWithOrderedAccessToSymbolsFilament, Vector>;

}  // namespace LabelNameSet

namespace LabelSet {

template <template <class> class Vector>
using DecodingTableFilament = Filaments::LabelSet<Symbol::DecodingTable, LabelNameSet::DecodingTable, Vector>;

template <template <class> class Vector>
using DecodingTable = BareBones::SnugComposite::DecodingTable<DecodingTableFilament, Vector>;

template <template <class> class Vector>
using EncodingBimapFilament = Filaments::LabelSet<Symbol::EncodingBimap, LabelNameSet::EncodingBimap, Vector>;

template <template <class> class Vector>
using EncodingBimap = BareBones::SnugComposite::EncodingBimap<EncodingBimapFilament, Vector>;

template <template <class> class Vector>
using ShrinkableEncodingBimap = BareBones::SnugComposite::ShrinkableEncodingBimap<EncodingBimapFilament, Vector>;

template <template <class> class Vector>
using ParallelEncodingBimapFilament = Filaments::LabelSet<Symbol::EncodingBimap, LabelNameSet::EncodingBimap, Vector>;

template <template <class> class Vector>
using ParallelEncodingBimap = BareBones::SnugComposite::ParallelEncodingBimap<EncodingBimapFilament, Vector>;

template <template <class> class Vector>
using OrderedEncodingBimapFilament = Filaments::LabelSet<Symbol::OrderedEncodingBimap, LabelNameSet::OrderedEncodingBimap, Vector>;

template <template <class> class Vector>
using OrderedEncodingBimap = BareBones::SnugComposite::OrderedEncodingBimap<OrderedEncodingBimapFilament, Vector>;

template <template <class> class Vector>
using OrderedIndexingTableFilament = Filaments::LabelSet<Symbol::EncodingBimapWithOrderedAccess, LabelNameSet::EncodingBimapWithOrderedAccessToSymbols, Vector>;

template <template <class> class Vector>
using OrderedIndexingTable = BareBones::SnugComposite::OrderedDecodingTable<OrderedIndexingTableFilament, Vector>;

}  // namespace LabelSet

}  // namespace PromPP::Primitives::SnugComposites