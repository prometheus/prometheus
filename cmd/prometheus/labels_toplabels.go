// Copyright 2025 The Prometheus Authors
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

//go:build toplabels

package main

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/index"
)

// countBlockSymbols reads given block index and counts how many time each string
// occurs on time series labels.
func countBlockSymbols(ctx context.Context, block *tsdb.Block) (map[string]int, error) {
	names := map[string]int{}

	ir, err := block.Index()
	if err != nil {
		return names, err
	}

	labelNames, err := ir.LabelNames(ctx)
	if err != nil {
		return names, err
	}

	for _, name := range labelNames {
		name = strings.Clone(name)

		if _, ok := names[name]; !ok {
			names[name] = 0
		}

		values, err := ir.LabelValues(ctx, name, nil)
		if err != nil {
			return names, err
		}
		for _, value := range values {
			value = strings.Clone(value)

			if _, ok := names[value]; !ok {
				names[value] = 0
			}

			p, err := ir.Postings(ctx, name, value)
			if err != nil {
				return names, err
			}

			refs, err := index.ExpandPostings(p)
			if err != nil {
				return names, err
			}

			names[name] += len(refs)
			names[value] += len(refs)
		}
	}
	return names, ir.Close()
}

type labelCost struct {
	name string
	cost int
}

// selectBlockStringsToMap takes a block and returns a list of strings that are most commonly
// present on all time series.
// List is sorted starting with the most frequent strings.
func selectBlockStringsToMap(block *tsdb.Block) ([]string, error) {
	names, err := countBlockSymbols(context.Background(), block)
	if err != nil {
		return nil, fmt.Errorf("failed to build list of common strings in block %s: %w", block.Meta().ULID, err)
	}

	costs := make([]labelCost, 0, len(names))
	for name, count := range names {
		costs = append(costs, labelCost{name: name, cost: (len(name) - 1) * count})
	}
	slices.SortFunc(costs, func(a, b labelCost) int {
		return cmp.Compare(b.cost, a.cost)
	})

	mappedLabels := make([]string, 0, 256)
	for i, c := range costs {
		if i >= 256 {
			break
		}
		mappedLabels = append(mappedLabels, c.name)
	}
	return mappedLabels, nil
}

func mapCommonLabelSymbols(db *tsdb.DB, logger *slog.Logger) error {
	var block *tsdb.Block
	for _, b := range db.Blocks() {
		if block == nil || b.MaxTime() > block.MaxTime() {
			block = b
		}
	}
	if block == nil {
		logger.Info("No tsdb blocks found, can't map common label strings")
		return nil
	}

	logger.Info(
		"Finding most common label strings in last block",
		slog.String("block", block.String()),
	)
	mappedLabels, err := selectBlockStringsToMap(block)
	if err != nil {
		return err
	}
	logger.Info("Mapped common label strings", slog.Int("count", len(mappedLabels)))
	labels.MapLabels(mappedLabels)
	return nil
}
