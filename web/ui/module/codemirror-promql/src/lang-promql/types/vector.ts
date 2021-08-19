// Copyright 2021 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

export enum VectorMatchCardinality {
  CardOneToOne = 'one-to-one',
  CardManyToOne = 'many-to-one',
  CardOneToMany = 'one-to-many',
  CardManyToMany = 'many-to-many',
}

export interface VectorMatching {
  // The cardinality of the two Vectors.
  card: VectorMatchCardinality;
  // MatchingLabels contains the labels which define equality of a pair of
  // elements from the Vectors.
  matchingLabels: string[];
  // On includes the given label names from matching,
  // rather than excluding them.
  on: boolean;
  // Include contains additional labels that should be included in
  // the result from the side with the lower cardinality.
  include: string[];
}
