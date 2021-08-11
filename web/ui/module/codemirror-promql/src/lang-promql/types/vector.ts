// The MIT License (MIT)
//
// Copyright (c) 2020 The Prometheus Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

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
