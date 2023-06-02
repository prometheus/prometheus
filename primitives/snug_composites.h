#pragma once

#include "snug_composites_filaments.h"

namespace PromPP {
namespace Primitives {
namespace SnugComposites {
namespace Symbol {
using DecodingTable = BareBones::SnugComposite::DecodingTable<Filaments::Symbol>;
using EncodingBimap = BareBones::SnugComposite::EncodingBimap<Filaments::Symbol>;
using ParallelEncodingBimap = BareBones::SnugComposite::ParallelEncodingBimap<Filaments::Symbol>;
using OrderedEncodingBimap = BareBones::SnugComposite::OrderedEncodingBimap<Filaments::Symbol>;
using EncodingBimapWithOrderedAccess = BareBones::SnugComposite::EncodingBimapWithOrderedAccess<Filaments::Symbol>;
using OrderedDecodingTable = BareBones::SnugComposite::OrderedDecodingTable<Filaments::Symbol>;
}  // namespace Symbol

namespace LabelNameSet {
using DecodingTable = BareBones::SnugComposite::DecodingTable<Filaments::LabelNameSet<Symbol::DecodingTable>>;
using EncodingBimap = BareBones::SnugComposite::EncodingBimap<Filaments::LabelNameSet<Symbol::EncodingBimap>>;
using OrderedEncodingBimap = BareBones::SnugComposite::OrderedEncodingBimap<Filaments::LabelNameSet<Symbol::OrderedEncodingBimap>>;
using EncodingBimapWithOrderedAccess =
    BareBones::SnugComposite::EncodingBimapWithOrderedAccess<Filaments::LabelNameSet<Symbol::EncodingBimapWithOrderedAccess>>;
using EncodingBimapWithOrderedAccessToSymbols = BareBones::SnugComposite::EncodingBimap<Filaments::LabelNameSet<Symbol::EncodingBimapWithOrderedAccess>>;

}  // namespace LabelNameSet

namespace LabelSet {
using DecodingTable = BareBones::SnugComposite::DecodingTable<Filaments::LabelSet<Symbol::DecodingTable, LabelNameSet::DecodingTable>>;
using EncodingBimap = BareBones::SnugComposite::EncodingBimap<Filaments::LabelSet<Symbol::EncodingBimap, LabelNameSet::EncodingBimap>>;
using ParallelEncodingBimap = BareBones::SnugComposite::ParallelEncodingBimap<Filaments::LabelSet<Symbol::EncodingBimap, LabelNameSet::EncodingBimap>>;
using OrderedEncodingBimap =
    BareBones::SnugComposite::OrderedEncodingBimap<Filaments::LabelSet<Symbol::OrderedEncodingBimap, LabelNameSet::OrderedEncodingBimap>>;
using OrderedIndexingTable = BareBones::SnugComposite::OrderedDecodingTable<
    Filaments::LabelSet<Symbol::EncodingBimapWithOrderedAccess, LabelNameSet::EncodingBimapWithOrderedAccessToSymbols>>;
}  // namespace LabelSet
}  // namespace SnugComposites
}  // namespace Primitives
}  // namespace PromPP
